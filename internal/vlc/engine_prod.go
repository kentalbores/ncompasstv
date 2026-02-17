//go:build linux && arm64

// Production backend: CGO bindings to libVLC for Raspberry Pi 5.
// Runs in kiosk mode (no GUI) with hardware-accelerated decoding.
// Uses libVLC's MediaList + ListPlayer for gapless playback.
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
			// === KIOSK MODE — no GUI whatsoever ===
			"--intf=dummy",               // No Qt/GUI interface at all
			"--no-interact",              // No user interaction prompts
			"--mouse-hide-timeout=0",     // Hide mouse cursor immediately
			"--no-video-title-show",      // No filename overlay
			"--no-osd",                   // No on-screen display
			"--no-dbus",                  // No D-Bus (headless)
			"--no-qt-privacy-ask",        // Skip Qt privacy dialog
			"--no-snapshot-preview",      // No snapshot UI

			// === VIDEO OUTPUT ===
			"--fullscreen",               // True fullscreen
			"--embedded-video",           // Embed video in VLC window
			"--no-video-deco",            // No window decorations

			// === HARDWARE DECODING (RPi5 VideoCore VII) ===
			"--avcodec-hw=any",           // V4L2 M2M / VAAPI auto-detect
			"--avcodec-threads=4",        // Use all 4 RPi5 cores for decoding
			"--avcodec-fast",             // Enable speed optimizations in decoder
			"--avcodec-skiploopfilter=0", // Keep deblocking (prevents blockiness)

			// === BUFFERING (critical for 4K 60fps) ===
			"--file-caching=8000",        // 8s file read-ahead (4K files are large)
			"--network-caching=3000",     // 3s network buffer
			"--live-caching=3000",        // 3s live buffer
			"--disc-caching=3000",        // 3s disc buffer

			// === FRAME TIMING ===
			// DO NOT use --no-drop-late-frames or --no-skip-frames on Pi.
			// The RPi5 GPU needs to manage its own frame pacing for 4K@60fps.
			// Forcing zero drops causes a frame backlog → stutter → quality collapse.
			"--clock-jitter=0",           // Tight clock
			"--deinterlace=0",            // Off (progressive 4K content)

			// === AUDIO ===
			"--aout=alsa",

			// === IMAGE ===
			"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

			"--quiet",
		}

		// Multi-zone: replace fullscreen with positioned window.
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

	log.Printf("[prod-backend:%s] initialized (kiosk mode, HW decode, 4-thread)", zone.ID)
	return nil
}

func (b *prodBackend) PlayAll(files []string, stopCh <-chan struct{}) error {
	if len(files) == 0 {
		return fmt.Errorf("empty playlist")
	}

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

	b.stopped = make(chan struct{})
	b.mu.Unlock()

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
