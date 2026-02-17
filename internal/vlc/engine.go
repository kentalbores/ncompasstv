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
//
// IMPORTANT: PlayAll must respond to Stop() without deadlocking.
// Backends must NOT hold a mutex while blocking.
type Backend interface {
	Init(zone template.Zone, screenW, screenH int) error
	PlayAll(files []string, stopCh <-chan struct{}) error
	Stop()
	Release()
}

// ZonePlayer manages a single zone's playback lifecycle.
type ZonePlayer struct {
	zone    template.Zone
	backend Backend
	mu      sync.Mutex
	files   []string
	running bool
	stopCh  chan struct{} // closed when the zone should shut down permanently
	restartCh chan struct{} // signaled when playlist changes during playback
}

// Engine coordinates all zone players for a given template.
type Engine struct {
	zones   []*ZonePlayer
	screenW int
	screenH int
}

// NewEngine creates an engine that manages playback across all zones.
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
			zone:      z,
			backend:   b,
			stopCh:    make(chan struct{}),
			restartCh: make(chan struct{}, 1),
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

// SetPlaylistAllZones sets the same playlist on all zones.
func (e *Engine) SetPlaylistAllZones(files []string) {
	for _, zp := range e.zones {
		zp.updatePlaylist(files)
	}
}

// Play starts all zone players in goroutines.
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
	log.Println("[engine] stopping all zones...")
	for _, zp := range e.zones {
		zp.stop()
	}
}

// Release frees all backend resources.
func (e *Engine) Release() {
	for _, zp := range e.zones {
		zp.stop()
		if zp.backend != nil {
			zp.backend.Release()
		}
	}
	log.Println("[engine] all zones released")
}

// Zones returns the list of zone IDs.
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
	zp.files = files
	wasRunning := zp.running
	zp.mu.Unlock()

	log.Printf("[zone:%s] playlist updated: %d files", zp.zone.ID, len(files))

	if wasRunning {
		// Signal the run loop to restart with the new playlist.
		zp.backend.Stop()
		select {
		case zp.restartCh <- struct{}{}:
		default:
		}
	}
}

func (zp *ZonePlayer) run() error {
	for {
		// Check for permanent shutdown.
		select {
		case <-zp.stopCh:
			log.Printf("[zone:%s] stopped", zp.zone.ID)
			return nil
		default:
		}

		zp.mu.Lock()
		files := make([]string, len(zp.files))
		copy(files, zp.files)
		zp.mu.Unlock()

		if len(files) == 0 {
			log.Printf("[zone:%s] no content, waiting...", zp.zone.ID)
			select {
			case <-zp.stopCh:
				return nil
			case <-zp.restartCh:
				continue
			case <-time.After(2 * time.Second):
				continue
			}
		}

		zp.mu.Lock()
		zp.running = true
		zp.mu.Unlock()

		log.Printf("[zone:%s] starting gapless playback (%d files)", zp.zone.ID, len(files))

		// PlayAll blocks until Stop() is called or it finishes.
		// Pass stopCh so the backend can listen for shutdown.
		err := zp.backend.PlayAll(files, zp.stopCh)

		zp.mu.Lock()
		zp.running = false
		zp.mu.Unlock()

		if err != nil {
			log.Printf("[zone:%s] playback error: %v", zp.zone.ID, err)
		}

		// Was this a permanent stop or a playlist restart?
		select {
		case <-zp.stopCh:
			log.Printf("[zone:%s] stopped", zp.zone.ID)
			return nil
		case <-zp.restartCh:
			log.Printf("[zone:%s] restarting with updated playlist", zp.zone.ID)
			continue
		default:
			// PlayAll returned on its own (error or VLC exit) — restart.
			time.Sleep(500 * time.Millisecond)
			continue
		}
	}
}

func (zp *ZonePlayer) stop() {
	// Close stopCh first so the run loop knows it's a permanent stop.
	select {
	case <-zp.stopCh:
		// Already closed.
	default:
		close(zp.stopCh)
	}

	// Then tell the backend to stop playback.
	zp.backend.Stop()

	zp.mu.Lock()
	zp.running = false
	zp.mu.Unlock()
}
