//go:build !(linux && arm64)

// Development backend: uses a single VLC subprocess per zone with its
// built-in playlist for gapless playback. No CGO required.
// VLC handles seamless transitions between videos and image display natively.
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
	log.Println("[dev-backend] *** Production builds (linux/arm64) use CGO/libVLC with MMAL ***")
	return nil
}

func (b *devBackend) PlayAll(files []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(files) == 0 {
		return fmt.Errorf("empty playlist")
	}

	// Count media types for logging.
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
	b.cmd = exec.Command(b.vlcPath, args...)
	b.cmd.Stdout = os.Stdout
	b.cmd.Stderr = os.Stderr

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("vlc start failed: %w", err)
	}

	// Wait blocks until VLC exits (user stop, crash, or playlist change).
	err := b.cmd.Wait()
	b.cmd = nil

	if err != nil {
		// Exit errors are normal when we kill the process on playlist change.
		return nil
	}
	return nil
}

func (b *devBackend) buildArgs(files []string) []string {
	args := []string{
		// --- Display ---
		"--fullscreen",
		"--no-video-title-show",
		"--no-osd",
		"--loop",
		"--no-random",

		// --- Hardware Decoding (critical for 4K) ---
		"--avcodec-hw=any",           // Auto-select best HW decoder (DXVA2/D3D11VA on Win, VAAPI on Linux)
		"--avcodec-threads=0",        // Auto-detect optimal thread count for CPU decoding fallback

		// --- Buffering (prevents stutter and quality drops) ---
		"--file-caching=5000",        // 5s file read-ahead buffer
		"--network-caching=3000",     // 3s network buffer
		"--live-caching=3000",        // 3s live stream buffer
		"--disc-caching=3000",        // 3s disc buffer
		"--clock-jitter=0",           // Disable clock jitter compensation
		"--clock-synchro=0",          // Disable clock sync (smoother playback)

		// --- Quality Preservation ---
		"--no-drop-late-frames",      // NEVER drop frames — prevents quality degradation
		"--no-skip-frames",           // NEVER skip frames — prevents blocky artifacts
		"--avcodec-skiploopfilter=0", // Keep deblocking filter (prevents blockiness)
		"--avcodec-fast",             // Use fast decoding paths where safe
		"--deinterlace=0",            // Disable deinterlacing (progressive 4K content)

		// --- Video Output (Windows: Direct3D11 for GPU compositing) ---
		"--vout=direct3d11",          // D3D11 output — GPU-composited, tear-free

		// --- Image Support ---
		"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

		"--quiet",
	}

	// Zone positioning — only add if not fullscreen (100x100 at 0,0).
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

	// Append all media files as VLC playlist items.
	args = append(args, files...)

	return args
}

func (b *devBackend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd = nil
	}
}

func (b *devBackend) Release() {
	b.Stop()
	log.Printf("[dev-backend:%s] released", b.zone.ID)
}

// findVLC locates the VLC executable on the system.
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
