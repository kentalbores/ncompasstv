//go:build linux && arm64

// Production backend: CGO bindings to libVLC with RPi5 hardware acceleration.
// Uses libVLC's MediaList + ListPlayer for true gapless playback per zone.
// RPi5 uses VideoCore VII — video output auto-detected (KMS/X11).
package vlc

import (
	"fmt"
	"log"
	"strconv"
	"sync"

	"player-native/internal/media"
	"player-native/internal/template"

	libvlc "github.com/adrg/libvlc-go/v3"
)

var vlcInitOnce sync.Once
var vlcInitErr error

type prodBackend struct {
	mu         sync.Mutex
	listPlayer *libvlc.ListPlayer
	mediaList  *libvlc.MediaList
	zone       template.Zone
	stopped    chan struct{}
}

func newBackend() (Backend, error) {
	return &prodBackend{}, nil
}

func (b *prodBackend) Init(zone template.Zone, screenW, screenH int) error {
	b.zone = zone
	b.stopped = make(chan struct{})

	vlcInitOnce.Do(func() {
		flags := []string{
			"--avcodec-hw=any",
			"--fullscreen",
			"--no-osd",
			"--no-dbus",
			"--no-video-title-show",
			"--aout=alsa",
			"--file-caching=5000",
			"--network-caching=3000",
			"--live-caching=3000",
			"--clock-jitter=0",
			"--clock-synchro=0",
			"--no-drop-late-frames",
			"--no-skip-frames",
			"--avcodec-skiploopfilter=0",
			"--deinterlace=0",
			"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),
			"--quiet",
		}

		if zone.Width < 100 || zone.Height < 100 || zone.X > 0 || zone.Y > 0 {
			pixelX := zone.X * screenW / 100
			pixelY := zone.Y * screenH / 100
			pixelW := zone.Width * screenW / 100
			pixelH := zone.Height * screenH / 100

			filtered := flags[:0]
			for _, f := range flags {
				if f != "--fullscreen" {
					filtered = append(filtered, f)
				}
			}
			flags = filtered
			flags = append(flags,
				"--video-x="+strconv.Itoa(pixelX),
				"--video-y="+strconv.Itoa(pixelY),
				"--width="+strconv.Itoa(pixelW),
				"--height="+strconv.Itoa(pixelH),
			)
		}

		vlcInitErr = libvlc.Init(flags...)
	})

	if vlcInitErr != nil {
		return fmt.Errorf("libvlc init failed: %w", vlcInitErr)
	}

	listPlayer, err := libvlc.NewListPlayer()
	if err != nil {
		return fmt.Errorf("list player creation failed: %w", err)
	}

	b.mu.Lock()
	b.listPlayer = listPlayer
	b.mu.Unlock()

	log.Printf("[prod-backend:%s] libVLC ListPlayer initialized (auto video output)", zone.ID)
	return nil
}

func (b *prodBackend) PlayAll(files []string, stopCh <-chan struct{}) error {
	if len(files) == 0 {
		return fmt.Errorf("empty playlist")
	}

	// Build media list (short lock, no blocking).
	b.mu.Lock()
	list, err := libvlc.NewMediaList()
	if err != nil {
		b.mu.Unlock()
		return fmt.Errorf("media list creation failed: %w", err)
	}

	for _, f := range files {
		m, err := libvlc.NewMediaFromPath(f)
		if err != nil {
			log.Printf("[prod-backend:%s] skip %s: %v", b.zone.ID, f, err)
			continue
		}
		if err := list.AddMedia(m); err != nil {
			log.Printf("[prod-backend:%s] skip add %s: %v", b.zone.ID, f, err)
			m.Release()
			continue
		}
	}

	if b.mediaList != nil {
		b.mediaList.Release()
	}
	b.mediaList = list

	if err := b.listPlayer.SetMediaList(list); err != nil {
		b.mu.Unlock()
		return fmt.Errorf("set media list failed: %w", err)
	}
	b.listPlayer.SetPlaybackMode(libvlc.Loop)

	var videos, images int
	for _, f := range files {
		switch media.Detect(f) {
		case media.Video:
			videos++
		case media.Image:
			images++
		}
	}
	log.Printf("[prod-backend:%s] starting gapless: %d videos + %d images",
		b.zone.ID, videos, images)

	if err := b.listPlayer.Play(); err != nil {
		b.mu.Unlock()
		return fmt.Errorf("play failed: %w", err)
	}

	// Reset the stopped channel.
	b.stopped = make(chan struct{})
	b.mu.Unlock()

	// Block until Stop() is called or permanent shutdown.
	// NO MUTEX HELD while blocking.
	select {
	case <-stopCh:
		b.listPlayer.Stop()
		return nil
	case <-b.stopped:
		return nil
	}
}

func (b *prodBackend) Stop() {
	b.mu.Lock()
	if b.listPlayer != nil {
		b.listPlayer.Stop()
	}
	// Signal PlayAll to unblock.
	select {
	case <-b.stopped:
	default:
		close(b.stopped)
	}
	b.mu.Unlock()
}

func (b *prodBackend) Release() {
	b.Stop()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.mediaList != nil {
		b.mediaList.Release()
		b.mediaList = nil
	}
	if b.listPlayer != nil {
		b.listPlayer.Release()
		b.listPlayer = nil
	}
	libvlc.Release()
	log.Printf("[prod-backend:%s] released", b.zone.ID)
}
