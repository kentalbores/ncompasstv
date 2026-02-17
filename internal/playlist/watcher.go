// Package playlist provides real-time folder monitoring for media
// directories, maintaining a sorted queue of media files that zone
// engines consume for playback.
package playlist

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"player-native/internal/media"

	"github.com/fsnotify/fsnotify"
)

// OnChangeFunc is a callback invoked when the playlist changes.
// It receives the updated sorted list of absolute file paths.
type OnChangeFunc func(files []string)

// Watcher monitors a directory for file system events and maintains
// a sorted list of playable media files (videos and images).
type Watcher struct {
	mu       sync.RWMutex
	dir      string
	files    []string
	watcher  *fsnotify.Watcher
	onChange OnChangeFunc
	stopCh   chan struct{}
}

// NewWatcher creates a new Watcher for the given directory.
// The onChange callback fires whenever the file list changes.
func NewWatcher(dir string, onChange OnChangeFunc) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		dir:      dir,
		watcher:  fw,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}

	// Perform initial scan before starting the watch loop.
	w.scan()

	return w, nil
}

// scan reads the directory and builds the sorted file list.
func (w *Watcher) scan() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		log.Printf("[watcher] scan error: %v", err)
		return
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if media.IsSupported(entry.Name()) {
			files = append(files, filepath.Join(w.dir, entry.Name()))
		}
	}

	sort.Strings(files)

	w.mu.Lock()
	w.files = files
	w.mu.Unlock()

	log.Printf("[watcher] scanned %d media files in %s", len(files), w.dir)
}

// Files returns the current sorted list of media file paths.
func (w *Watcher) Files() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	dst := make([]string, len(w.files))
	copy(dst, w.files)
	return dst
}

// Start begins watching the directory for changes. It blocks until
// Stop() is called or the watcher encounters a fatal error.
func (w *Watcher) Start() error {
	if err := w.watcher.Add(w.dir); err != nil {
		return err
	}

	log.Printf("[watcher] monitoring: %s", w.dir)

	for {
		select {
		case <-w.stopCh:
			log.Println("[watcher] stopped")
			return nil

		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			if isRelevantEvent(event) {
				log.Printf("[watcher] event: %s %s", event.Op, event.Name)
				w.scan()
				if w.onChange != nil {
					w.onChange(w.Files())
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("[watcher] error: %v", err)
		}
	}
}

// Stop halts the watcher loop and releases the fsnotify resources.
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.watcher.Close()
}

// isRelevantEvent filters for file create, remove, and rename events
// that would change the playlist contents.
func isRelevantEvent(e fsnotify.Event) bool {
	return e.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
}
