// Package api handles remote server communication, including heartbeat
// reporting and identity management via a legacy config.json format.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// Config mirrors the legacy Node.js config.json identity structure.
type Config struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Interval int    `json:"heartbeat_interval_sec"`
}

// Heartbeat is the payload sent to the remote server on each tick.
type Heartbeat struct {
	ID        string  `json:"id"`
	Key       string  `json:"key"`
	Timestamp string  `json:"timestamp"`
	Uptime    float64 `json:"uptime_sec"`
	Version   string  `json:"version"`
	Arch      string  `json:"arch"`
	OS        string  `json:"os"`
}

// Client manages the heartbeat loop and server communication.
type Client struct {
	mu      sync.RWMutex
	cfg     Config
	cfgPath string
	version string
	startAt time.Time
	httpCli *http.Client
	stopCh  chan struct{}
}

// DefaultConfigPath is the standard location for the player identity file.
const DefaultConfigPath = "/etc/player/config.json"

// NewClient creates an API client by loading the config from the given path.
// If the file does not exist, the client starts in "unregistered" mode
// and will log warnings on each heartbeat attempt until configured.
func NewClient(cfgPath, version string) (*Client, error) {
	c := &Client{
		cfgPath: cfgPath,
		version: version,
		startAt: time.Now(),
		httpCli: &http.Client{Timeout: 10 * time.Second},
		stopCh:  make(chan struct{}),
	}

	if err := c.loadConfig(); err != nil {
		log.Printf("[api] config load warning: %v (running unregistered)", err)
	}

	return c, nil
}

// loadConfig reads and parses the config.json file.
func (c *Client) loadConfig() error {
	data, err := os.ReadFile(c.cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 60 // default 60s heartbeat
	}

	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()

	log.Printf("[api] loaded config: id=%s endpoint=%s interval=%ds", cfg.ID, cfg.Endpoint, cfg.Interval)
	return nil
}

// ReloadConfig re-reads the config from disk. Safe to call at runtime.
func (c *Client) ReloadConfig() error {
	return c.loadConfig()
}

// StartHeartbeat begins the periodic heartbeat loop.
// It blocks until Stop() is called.
func (c *Client) StartHeartbeat() {
	c.mu.RLock()
	interval := time.Duration(c.cfg.Interval) * time.Second
	if interval == 0 {
		interval = 60 * time.Second
	}
	c.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[api] heartbeat started (every %s)", interval)

	// Send one immediately on start.
	c.sendHeartbeat()

	for {
		select {
		case <-c.stopCh:
			log.Println("[api] heartbeat stopped")
			return
		case <-ticker.C:
			c.sendHeartbeat()
		}
	}
}

// sendHeartbeat constructs and POSTs a heartbeat to the configured endpoint.
func (c *Client) sendHeartbeat() {
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()

	if cfg.Endpoint == "" || cfg.ID == "" {
		log.Println("[api] heartbeat skipped: missing endpoint or id")
		return
	}

	hb := Heartbeat{
		ID:        cfg.ID,
		Key:       cfg.Key,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(c.startAt).Seconds(),
		Version:   c.version,
		Arch:      runtime.GOARCH,
		OS:        runtime.GOOS,
	}

	body, err := json.Marshal(hb)
	if err != nil {
		log.Printf("[api] heartbeat marshal error: %v", err)
		return
	}

	url := fmt.Sprintf("%s/heartbeat", cfg.Endpoint)
	resp, err := c.httpCli.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[api] heartbeat POST failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("[api] heartbeat response: %d", resp.StatusCode)
		return
	}

	log.Printf("[api] heartbeat sent OK (%d)", resp.StatusCode)
}

// GetConfig returns the current configuration (thread-safe).
func (c *Client) GetConfig() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// Stop halts the heartbeat loop.
func (c *Client) Stop() {
	close(c.stopCh)
}
