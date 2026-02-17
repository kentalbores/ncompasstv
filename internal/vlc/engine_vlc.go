// Unified VLC backend using subprocess. Works on all platforms.
// Each zone gets its own VLC process with per-zone window configuration.
//
// This approach:
//   - Gives per-zone window control (position, size, decorations)
//   - Gapless playback via --loop
//   - Hardware acceleration via --avcodec-hw=any
//   - No CGO needed (cross-compilable)
//   - --no-video-deco works because Qt interface is loaded in CLI VLC
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

type vlcBackend struct {
	mu         sync.Mutex
	vlcPath    string
	cmd        *exec.Cmd
	zone       template.Zone
	screenW    int
	screenH    int
	isFullZone bool
}

func newBackend() (Backend, error) {
	return &vlcBackend{}, nil
}

func (b *vlcBackend) Init(zone template.Zone, screenW, screenH int) error {
	path, err := findVLC()
	if err != nil {
		return err
	}
	b.vlcPath = path
	b.zone = zone
	b.screenW = screenW
	b.screenH = screenH
	b.isFullZone = zone.X == 0 && zone.Y == 0 && zone.Width >= 100 && zone.Height >= 100

	log.Printf("[vlc:%s] using %s (screen %dx%d, fullzone=%v)", zone.ID, path, screenW, screenH, b.isFullZone)
	return nil
}

func (b *vlcBackend) PlayAll(files []string, stopCh <-chan struct{}) error {
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
	log.Printf("[vlc:%s] playing %d videos + %d images (looped)", b.zone.ID, videos, images)

	args := b.buildArgs(files)

	b.mu.Lock()
	b.cmd = exec.Command(b.vlcPath, args...)
	b.cmd.Stdout = os.Stdout
	b.cmd.Stderr = os.Stderr
	cmd := b.cmd
	b.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("vlc start failed: %w", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	select {
	case <-stopCh:
		b.kill()
		return nil
	case <-doneCh:
		return nil
	}
}

func (b *vlcBackend) buildArgs(files []string) []string {
	args := []string{
		// === KIOSK: nothing visible except video ===
		"--no-video-deco",        // No window title bar or borders
		"--video-on-top",         // Always on top of everything
		"--no-video-title-show",  // No filename overlay
		"--no-osd",               // No on-screen display
		"--no-spu",               // No subtitles
		"--mouse-hide-timeout=0", // Hide cursor immediately
		"--no-qt-fs-controller",  // No fullscreen controller bar
		"--no-qt-name-in-title",  // No "VLC" in title
		"--no-qt-privacy-ask",    // Skip privacy dialog

		// === PLAYBACK ===
		"--loop",      // Loop playlist forever
		"--no-random", // Maintain order

		// === HARDWARE DECODING ===
		"--avcodec-hw=any",           // Auto-detect (DXVA2, V4L2 M2M, VAAPI)
		"--avcodec-threads=0",        // Auto-detect core count
		"--avcodec-skiploopfilter=0", // Keep deblocking filter

		// === BUFFERING ===
		"--file-caching=8000",    // 8s file read-ahead (4K files are large)
		"--network-caching=3000", // 3s network buffer
		"--live-caching=3000",    // 3s live buffer
		"--disc-caching=3000",    // 3s disc buffer

		// === TIMING ===
		"--clock-jitter=0", // Tight clock sync
		"--deinterlace=0",  // Off (4K content is progressive)

		// === IMAGE ===
		"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

		"--quiet",
	}

	// Platform-specific output modules.
	switch runtime.GOOS {
	case "windows":
		args = append(args, "--vout=direct3d11")
	case "linux":
		args = append(args, "--aout=alsa")
	}

	// Zone positioning: fullscreen OR exact window placement.
	if b.isFullZone {
		args = append(args, "--fullscreen")
	} else {
		pixelX := b.zone.X * b.screenW / 100
		pixelY := b.zone.Y * b.screenH / 100
		pixelW := b.zone.Width * b.screenW / 100
		pixelH := b.zone.Height * b.screenH / 100

		args = append(args,
			"--width="+strconv.Itoa(pixelW),
			"--height="+strconv.Itoa(pixelH),
			"--video-x="+strconv.Itoa(pixelX),
			"--video-y="+strconv.Itoa(pixelY),
		)
	}

	args = append(args, files...)
	return args
}

func (b *vlcBackend) Stop() {
	b.kill()
}

func (b *vlcBackend) Release() {
	b.kill()
	log.Printf("[vlc:%s] released", b.zone.ID)
}

func (b *vlcBackend) kill() {
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

	var candidates []string
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
