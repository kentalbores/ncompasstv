//go:build !(linux && arm64)

// Development backend: uses a single VLC subprocess per zone with its
// built-in playlist for gapless playback. No CGO required.
package vlc

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"player-native/internal/media"
	"player-native/internal/template"
)

type devBackend struct {
	mu      sync.Mutex
	vlcPath string
	cmd     *exec.Cmd
	zone    template.Zone
	screenW int
	screenH int
}

func newBackend() (Backend, error) {
	return &devBackend{}, nil
}

func (b *devBackend) Init(zone template.Zone, screenW, screenH int) error {
	path, err := findVLC()
	if err != nil {
		return err
	}
	b.vlcPath = path
	b.zone = zone
	b.screenW = screenW
	b.screenH = screenH

	log.Printf("[dev-backend:%s] using VLC at: %s", zone.ID, path)
	log.Println("[dev-backend] *** DEVELOPMENT MODE — gapless via VLC subprocess ***")
	return nil
}

func (b *devBackend) PlayAll(files []string, stopCh <-chan struct{}) error {
	if len(files) == 0 {
		return fmt.Errorf("empty playlist")
	}

	var videos, images int
	for _, f := range files {
		switch media.Detect(f) {
		case media.Video:
			videos++
		case media.Image:
			images++
		}
	}
	log.Printf("[dev-backend:%s] playing %d videos + %d images (gapless, looped)",
		b.zone.ID, videos, images)

	args := b.buildArgs(files)

	// Start VLC (short lock).
	b.mu.Lock()
	b.cmd = exec.Command(b.vlcPath, args...)
	b.cmd.Stdout = os.Stdout
	b.cmd.Stderr = os.Stderr
	cmd := b.cmd
	b.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("vlc start failed: %w", err)
	}

	// Wait for VLC to exit in a goroutine.
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	// Block until VLC exits OR we're told to stop. NO MUTEX HELD.
	select {
	case <-stopCh:
		b.kill()
		return nil
	case <-doneCh:
		return nil
	}
}

func (b *devBackend) buildArgs(files []string) []string {
	args := []string{
		"--fullscreen",
		"--no-video-title-show",
		"--no-osd",
		"--loop",
		"--no-random",
		"--avcodec-hw=any",
		"--avcodec-threads=0",
		"--file-caching=5000",
		"--network-caching=3000",
		"--live-caching=3000",
		"--disc-caching=3000",
		"--clock-jitter=0",
		"--clock-synchro=0",
		"--no-drop-late-frames",
		"--no-skip-frames",
		"--avcodec-skiploopfilter=0",
		"--avcodec-fast",
		"--deinterlace=0",
		"--vout=direct3d11",
		"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),
		"--quiet",
	}

	if b.zone.Width < 100 || b.zone.Height < 100 || b.zone.X > 0 || b.zone.Y > 0 {
		pixelX := b.zone.X * b.screenW / 100
		pixelY := b.zone.Y * b.screenH / 100
		pixelW := b.zone.Width * b.screenW / 100
		pixelH := b.zone.Height * b.screenH / 100

		args = append(args,
			"--no-fullscreen",
			"--no-embedded-video",
			"--no-qt-fs-controller",
			"--video-x="+strconv.Itoa(pixelX),
			"--video-y="+strconv.Itoa(pixelY),
			"--width="+strconv.Itoa(pixelW),
			"--height="+strconv.Itoa(pixelH),
		)
	}

	args = append(args, files...)
	return args
}

func (b *devBackend) Stop() {
	b.kill()
}

func (b *devBackend) Release() {
	b.kill()
	log.Printf("[dev-backend:%s] released", b.zone.ID)
}

func (b *devBackend) kill() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd = nil
	}
}

func findVLC() (string, error) {
	if path, err := exec.LookPath("vlc"); err == nil {
		return path, nil
	}

	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\VideoLAN\VLC\vlc.exe`,
			`C:\Program Files (x86)\VideoLAN\VLC\vlc.exe`,
		}
	case "darwin":
		candidates = []string{
			"/Applications/VLC.app/Contents/MacOS/VLC",
		}
	default:
		candidates = []string{
			"/usr/bin/vlc",
			"/snap/bin/vlc",
		}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("VLC not found — install from https://www.videolan.org/vlc/")
}
