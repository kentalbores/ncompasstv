package playlist

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestScanFindsMediaFiles verifies that scan() picks up supported
// video and image extensions and sorts them alphabetically.
func TestScanFindsMediaFiles(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"charlie.mp4",
		"alpha.mkv",
		"bravo.avi",
		"notes.txt",      // should be ignored
		"readme.md",      // should be ignored
		"delta.hevc",
		"echo.webm",
		"foxtrot.jpg",    // image — should be included
		"golf.png",       // image — should be included
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	w, err := NewWatcher(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	got := w.Files()

	// Expect 7 media files (5 video + 2 image), sorted alphabetically.
	expected := []string{
		filepath.Join(dir, "alpha.mkv"),
		filepath.Join(dir, "bravo.avi"),
		filepath.Join(dir, "charlie.mp4"),
		filepath.Join(dir, "delta.hevc"),
		filepath.Join(dir, "echo.webm"),
		filepath.Join(dir, "foxtrot.jpg"),
		filepath.Join(dir, "golf.png"),
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(got), got)
	}

	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("index %d: expected %s, got %s", i, expected[i], got[i])
		}
	}
}

// TestScanIgnoresDirectories ensures subdirectories are not included.
func TestScanIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()

	os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("test"), 0644)

	w, err := NewWatcher(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	got := w.Files()
	if len(got) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(got), got)
	}
}

// TestScanEmptyDir handles an empty playlist directory gracefully.
func TestScanEmptyDir(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	got := w.Files()
	if len(got) != 0 {
		t.Fatalf("expected 0 files, got %d", len(got))
	}
}

// TestWatcherDetectsNewFile verifies the onChange callback fires
// when a new media file is added to the watched directory.
func TestWatcherDetectsNewFile(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var lastFiles []string
	changed := make(chan struct{}, 1)

	w, err := NewWatcher(dir, func(files []string) {
		mu.Lock()
		lastFiles = files
		mu.Unlock()
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	go w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	os.WriteFile(filepath.Join(dir, "new_video.mp4"), []byte("data"), 0644)

	select {
	case <-changed:
		mu.Lock()
		defer mu.Unlock()
		if len(lastFiles) != 1 {
			t.Fatalf("expected 1 file after add, got %d", len(lastFiles))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for onChange callback")
	}
}

// TestWatcherDetectsRemoval verifies the callback fires when a file is removed.
func TestWatcherDetectsRemoval(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "existing.mp4")
	os.WriteFile(testFile, []byte("data"), 0644)

	changed := make(chan []string, 2)

	w, err := NewWatcher(dir, func(files []string) {
		changed <- files
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(w.Files()) != 1 {
		t.Fatalf("expected 1 file initially, got %d", len(w.Files()))
	}

	go w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	os.Remove(testFile)

	select {
	case files := <-changed:
		if len(files) != 0 {
			t.Fatalf("expected 0 files after removal, got %d", len(files))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for removal callback")
	}
}

// TestWatcherDetectsImageFile verifies images are picked up alongside videos.
func TestWatcherDetectsImageFile(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var lastFiles []string
	changed := make(chan struct{}, 2)

	w, err := NewWatcher(dir, func(files []string) {
		mu.Lock()
		lastFiles = files
		mu.Unlock()
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	go w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	os.WriteFile(filepath.Join(dir, "banner.png"), []byte("img"), 0644)

	select {
	case <-changed:
		mu.Lock()
		defer mu.Unlock()
		if len(lastFiles) != 1 {
			t.Fatalf("expected 1 file, got %d", len(lastFiles))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for image detection")
	}
}
