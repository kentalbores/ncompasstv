// Package vlc provides a zone-aware video playback engine.
// Each zone runs an independent gapless playback loop (videos + images).
// On RPi5 (linux/arm64) it uses CGO with libVLC for DRM/KMS rendering.
// On other platforms it uses VLC as a subprocess for development testing.
package vlc

import (
	"log"
	"sync"
	"time"

	"player-native/internal/template"
)

// Backend is the platform-specific playback implementation for a single zone.
// Each zone gets its own Backend instance.
type Backend interface {
	// Init prepares the backend for playback in the given zone geometry.
	Init(zone template.Zone, screenW, screenH int) error

	// PlayAll starts gapless looped playback of the given file list.
	// It blocks until Stop() is called or an unrecoverable error occurs.
	// When the playlist changes, the caller calls PlayAll again after Stop.
	PlayAll(files []string) error

	// Stop halts the current playback. Must return quickly.
	Stop()

	// Release frees all resources held by this backend.
	Release()
}

// ZonePlayer manages a single zone's playback lifecycle.
type ZonePlayer struct {
	zone    template.Zone
	backend Backend
	mu      sync.Mutex
	files   []string
	running bool
	stopCh  chan struct{}
}

// Engine coordinates all zone players for a given template.
type Engine struct {
	zones   []*ZonePlayer
	screenW int
	screenH int
}

// NewEngine creates an engine that manages playback across all zones
// in the given template. Each zone gets its own independent backend.
func NewEngine(tmpl *template.Template, screenW, screenH int) (*Engine, error) {
	e := &Engine{
		screenW: screenW,
		screenH: screenH,
	}

	for _, z := range tmpl.Zones {
		b, err := newBackend()
		if err != nil {
			e.Release()
			return nil, err
		}

		if err := b.Init(z, screenW, screenH); err != nil {
			e.Release()
			return nil, err
		}

		zp := &ZonePlayer{
			zone:    z,
			backend: b,
			stopCh:  make(chan struct{}),
		}
		e.zones = append(e.zones, zp)
		log.Printf("[engine] zone %q initialized (%d%%x%d%% at %d%%,%d%%)",
			z.ID, z.Width, z.Height, z.X, z.Y)
	}

	log.Printf("[engine] %d zone(s) ready", len(e.zones))
	return e, nil
}

// SetPlaylist updates the file list for a specific zone by ID.
func (e *Engine) SetPlaylist(zoneID string, files []string) {
	for _, zp := range e.zones {
		if zp.zone.ID == zoneID {
			zp.updatePlaylist(files)
			return
		}
	}
	log.Printf("[engine] warning: zone %q not found", zoneID)
}

// SetPlaylistAllZones sets the same playlist on all zones (convenience for single-zone).
func (e *Engine) SetPlaylistAllZones(files []string) {
	for _, zp := range e.zones {
		zp.updatePlaylist(files)
	}
}

// Play starts all zone players. Each zone runs independently in its
// own goroutine. Returns a channel that receives an error if any zone fails.
func (e *Engine) Play() <-chan error {
	errCh := make(chan error, len(e.zones))
	for _, zp := range e.zones {
		go func(z *ZonePlayer) {
			errCh <- z.run()
		}(zp)
	}
	return errCh
}

// Stop halts all zone players.
func (e *Engine) Stop() {
	for _, zp := range e.zones {
		zp.stop()
	}
}

// Release frees all backend resources across all zones.
func (e *Engine) Release() {
	for _, zp := range e.zones {
		zp.stop()
		if zp.backend != nil {
			zp.backend.Release()
		}
	}
	log.Println("[engine] all zones released")
}

// Zones returns the list of zone IDs managed by this engine.
func (e *Engine) Zones() []string {
	ids := make([]string, len(e.zones))
	for i, zp := range e.zones {
		ids[i] = zp.zone.ID
	}
	return ids
}

// --- ZonePlayer internals ---

func (zp *ZonePlayer) updatePlaylist(files []string) {
	zp.mu.Lock()
	defer zp.mu.Unlock()

	zp.files = files
	log.Printf("[zone:%s] playlist updated: %d files", zp.zone.ID, len(files))

	// If currently playing, restart with the new playlist.
	if zp.running {
		zp.backend.Stop()
	}
}

func (zp *ZonePlayer) run() error {
	for {
		zp.mu.Lock()
		files := make([]string, len(zp.files))
		copy(files, zp.files)
		zp.mu.Unlock()

		if len(files) == 0 {
			log.Printf("[zone:%s] no content, waiting...", zp.zone.ID)
			select {
			case <-zp.stopCh:
				return nil
			case <-time.After(1 * time.Second):
				continue
			}
		}

		zp.mu.Lock()
		zp.running = true
		zp.mu.Unlock()

		log.Printf("[zone:%s] starting gapless playback (%d files)", zp.zone.ID, len(files))
		err := zp.backend.PlayAll(files)

		zp.mu.Lock()
		zp.running = false
		zp.mu.Unlock()

		if err != nil {
			log.Printf("[zone:%s] playback error: %v", zp.zone.ID, err)
		}

		// Check if we should stop or loop with (potentially updated) playlist.
		select {
		case <-zp.stopCh:
			return nil
		default:
			continue
		}
	}
}

func (zp *ZonePlayer) stop() {
	zp.mu.Lock()
	defer zp.mu.Unlock()
	if zp.running {
		zp.running = false
		zp.backend.Stop()
	}
	select {
	case <-zp.stopCh:
	default:
		close(zp.stopCh)
	}
}
