// Unified VLC backend using subprocess.
//
// Linux:   Uses cvlc (VLC without Qt GUI) + xdotool for window positioning.
//          cvlc shows ONLY the video — no menus, no controls, no decorations.
//          xdotool sets override-redirect which removes the window from WM
//          control entirely (no title bar, no taskbar entry, exact positioning).
//
// Windows: Uses vlc.exe with Qt kiosk flags for development testing.
package vlc

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

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

	if runtime.GOOS == "linux" {
		b.cmd.Env = append(os.Environ(),
			"DISPLAY=:0",
		)
	}

	cmd := b.cmd
	b.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("vlc start failed: %w", err)
	}

	// On Linux, use xdotool to position the window after it appears.
	if runtime.GOOS == "linux" && cmd.Process != nil {
		go b.positionWindow(cmd.Process.Pid)
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
	if runtime.GOOS == "linux" {
		return b.buildLinuxArgs(files)
	}
	return b.buildWindowsArgs(files)
}

// buildLinuxArgs builds flags for cvlc (no Qt interface).
// Window positioning is handled by xdotool, not VLC flags.
func (b *vlcBackend) buildLinuxArgs(files []string) []string {
	args := []string{
		"--no-video-title-show", // No filename overlay
		"--no-osd",              // No on-screen display
		"--no-spu",              // No subtitles

		"--loop",      // Loop playlist
		"--no-random", // Maintain order

		"--avcodec-hw=any",           // HW decode (V4L2 M2M on RPi5)
		"--avcodec-threads=0",        // Auto-detect cores
		"--avcodec-skiploopfilter=0", // Keep deblocking

		"--file-caching=8000",
		"--network-caching=3000",
		"--live-caching=3000",
		"--disc-caching=3000",

		"--clock-jitter=0",
		"--deinterlace=0",

		"--aout=alsa",

		"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

		"--quiet",
	}

	args = append(args, files...)
	return args
}

// buildWindowsArgs builds flags for vlc.exe (Qt interface with kiosk flags).
func (b *vlcBackend) buildWindowsArgs(files []string) []string {
	args := []string{
		"--no-video-deco",
		"--video-on-top",
		"--no-video-title-show",
		"--no-osd",
		"--no-spu",
		"--mouse-hide-timeout=0",
		"--no-qt-fs-controller",
		"--no-qt-name-in-title",
		"--no-qt-privacy-ask",

		"--loop",
		"--no-random",

		"--avcodec-hw=any",
		"--avcodec-threads=0",
		"--avcodec-skiploopfilter=0",

		"--file-caching=8000",
		"--network-caching=3000",
		"--live-caching=3000",
		"--disc-caching=3000",

		"--clock-jitter=0",
		"--deinterlace=0",

		"--vout=direct3d11",

		"--image-duration=" + strconv.Itoa(media.DefaultImageDuration),

		"--quiet",
	}

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

// positionWindow uses xdotool to position the VLC video window.
// override-redirect removes the window from WM control entirely:
//   - No title bar or borders (WM doesn't draw decorations)
//   - No taskbar entry
//   - Exact pixel positioning
//   - Stays exactly where placed
func (b *vlcBackend) positionWindow(pid int) {
	pixelW := b.zone.Width * b.screenW / 100
	pixelH := b.zone.Height * b.screenH / 100
	pixelX := b.zone.X * b.screenW / 100
	pixelY := b.zone.Y * b.screenH / 100

	if b.isFullZone {
		pixelW = b.screenW
		pixelH = b.screenH
		pixelX = 0
		pixelY = 0
	}

	pidStr := strconv.Itoa(pid)
	wStr := strconv.Itoa(pixelW)
	hStr := strconv.Itoa(pixelH)
	xStr := strconv.Itoa(pixelX)
	yStr := strconv.Itoa(pixelY)

	for attempt := 0; attempt < 50; attempt++ {
		time.Sleep(200 * time.Millisecond)

		out, err := exec.Command("xdotool", "search", "--pid", pidStr).Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			continue
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		windowID := lines[len(lines)-1]

		exec.Command("xdotool", "set_window", "--overrideredirect", "1", windowID).Run()
		exec.Command("xdotool", "windowsize", windowID, wStr, hStr).Run()
		exec.Command("xdotool", "windowmove", windowID, xStr, yStr).Run()
		exec.Command("xdotool", "windowactivate", windowID).Run()
		exec.Command("xdotool", "windowraise", windowID).Run()

		log.Printf("[vlc:%s] window %s positioned at %s,%s %sx%s (override-redirect)",
			b.zone.ID, windowID, xStr, yStr, wStr, hStr)
		return
	}
	log.Printf("[vlc:%s] warning: could not find window for PID %d after 10s", b.zone.ID, pid)
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
	// On Linux, prefer cvlc (VLC without Qt GUI — just video, no menus/controls).
	if runtime.GOOS == "linux" {
		for _, name := range []string{"cvlc", "/usr/bin/cvlc"} {
			if path, err := exec.LookPath(name); err == nil {
				return path, nil
			}
			if _, err := os.Stat(name); err == nil {
				return name, nil
			}
		}
	}

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
			"/usr/bin/cvlc",
			"/usr/bin/vlc",
			"/snap/bin/vlc",
		}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("VLC not found — install with: sudo apt install vlc")
}
