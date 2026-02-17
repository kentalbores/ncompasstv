//go:build linux && arm64

// Production backend: CGO bindings to libVLC with RPi5 MMAL hardware acceleration.
// Uses libVLC's MediaList + ListPlayer for true gapless playback per zone.
// This file only compiles on linux/arm64 (the Raspberry Pi 5 target).
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

// prodBackend wraps libVLC's ListPlayer for gapless hardware-accelerated
// playback on the Raspberry Pi 5.
type prodBackend struct {
	mu         sync.Mutex
	listPlayer *libvlc.ListPlayer
	mediaList  *libvlc.MediaList
	zone       template.Zone
	playing    bool
}

func newBackend() (Backend, error) {
	return &prodBackend{}, nil
}

func (b *prodBackend) Init(zone template.Zone, screenW, screenH int) error {
	b.zone = zone

	// Initialize libVLC once globally (shared across all zones).
	vlcInitOnce.Do(func() {
		flags := []string{
			// --- RPi5 Hardware Acceleration ---
			"--vout=mmal_vout",           // MMAL video output — bypasses desktop compositor
			"--codec=mmal_decoder",       // MMAL hardware decoder for H.264/HEVC
			"--no-xlib",                  // Skip X11 — render via DRM/KMS directly

			// --- Display ---
			"--no-osd",
			"--no-dbus",
			"--no-video-title-show",

			// --- Audio ---
			"--aout=alsa",

			// --- Buffering (prevents stutter on 4K streams) ---
			"--file-caching=5000",        // 5s file read-ahead buffer
			"--network-caching=3000",     // 3s network buffer
			"--live-caching=3000",        // 3s live stream buffer
			"--clock-jitter=0",           // Disable clock jitter compensation
			"--clock-synchro=0",          // Disable clock sync (smoother playback)

			// --- Quality Preservation ---
			"--no-drop-late-frames",      // NEVER drop frames
			"--no-skip-frames",           // NEVER skip frames
			"--avcodec-skiploopfilter=0", // Keep deblocking filter active
			"--deinterlace=0",            // Disable deinterlacing (progressive 4K content)

			// --- Image Support ---
			"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

			"--quiet",
		}

		// For non-fullscreen zones, add position/size overrides.
		if zone.Width < 100 || zone.Height < 100 || zone.X > 0 || zone.Y > 0 {
			pixelX := zone.X * screenW / 100
			pixelY := zone.Y * screenH / 100
			pixelW := zone.Width * screenW / 100
			pixelH := zone.Height * screenH / 100
			flags = append(flags,
				"--video-x="+strconv.Itoa(pixelX),
				"--video-y="+strconv.Itoa(pixelY),
				"--width="+strconv.Itoa(pixelW),
				"--height="+strconv.Itoa(pixelH),
			)
		} else {
			flags = append(flags, "--fullscreen")
		}

		vlcInitErr = libvlc.Init(flags...)
	})

	if vlcInitErr != nil {
		return fmt.Errorf("libvlc init failed: %w", vlcInitErr)
	}

	// Create a ListPlayer for gapless playlist playback.
	listPlayer, err := libvlc.NewListPlayer()
	if err != nil {
		return fmt.Errorf("list player creation failed: %w", err)
	}
	b.listPlayer = listPlayer

	log.Printf("[prod-backend:%s] libVLC ListPlayer initialized with MMAL", zone.ID)
	return nil
}

func (b *prodBackend) PlayAll(files []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(files) == 0 {
		return fmt.Errorf("empty playlist")
	}

	// Create a new media list from the file paths.
	list, err := libvlc.NewMediaList()
	if err != nil {
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

	// Release old media list if any.
	if b.mediaList != nil {
		b.mediaList.Release()
	}
	b.mediaList = list

	// Assign the list and set loop mode.
	if err := b.listPlayer.SetMediaList(list); err != nil {
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
		return fmt.Errorf("play failed: %w", err)
	}
	b.playing = true

	// Block until stopped externally.
	// ListPlayer loops internally — we just wait.
	select {}
}

func (b *prodBackend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.playing {
		b.listPlayer.Stop()
		b.playing = false
	}
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
