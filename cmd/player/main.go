// n-compasstv: Hardware-accelerated 4K digital signage player for Raspberry Pi 5.
// Supports multi-zone templates, gapless video playback, and image display.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"player-native/internal/api"
	"player-native/internal/playlist"
	"player-native/internal/system"
	"player-native/internal/template"
	"player-native/internal/vlc"

	"github.com/spf13/cobra"
)

// Build-time variables set by the Makefile via -ldflags.
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "n-compasstv",
		Short: "n-compasstv — 4K hardware-accelerated digital signage player",
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(checkCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runCmd is the primary command that starts the playback engine,
// folder watchers (one per zone), and heartbeat client.
func runCmd() *cobra.Command {
	var (
		playlistDir  string
		configPath   string
		templatePath string
		screenW      int
		screenH      int
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the video player engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetFlags(log.LstdFlags | log.Lmicroseconds)
			log.Printf("n-compasstv %s (built %s)", version, buildTime)

			// --- Load Template ---
			var tmpl *template.Template
			if templatePath != "" {
				var err error
				tmpl, err = template.LoadFromFile(templatePath)
				if err != nil {
					return fmt.Errorf("template load: %w", err)
				}
				log.Printf("[main] loaded template %q with %d zone(s)", tmpl.Name, len(tmpl.Zones))
			} else {
				tmpl = template.Fullscreen(playlistDir)
				log.Printf("[main] using default fullscreen template")
			}

			// Ensure all playlist directories exist.
			for i := range tmpl.Zones {
				z := &tmpl.Zones[i]
				if templatePath == "" {
					z.PlaylistDir = playlistDir
				}
				if err := system.EnsureDir(z.PlaylistDir); err != nil {
					return fmt.Errorf("playlist dir %s: %w", z.PlaylistDir, err)
				}
			}

			// --- Engine (manages all zones) ---
			engine, err := vlc.NewEngine(tmpl, screenW, screenH)
			if err != nil {
				return fmt.Errorf("engine init: %w", err)
			}
			defer engine.Release()

			// --- Per-Zone Playlist Watchers ---
			var watchers []*playlist.Watcher
			for _, z := range tmpl.Zones {
				zoneID := z.ID
				dir := z.PlaylistDir

				w, err := playlist.NewWatcher(dir, func(files []string) {
					log.Printf("[main] zone %q playlist changed: %d files", zoneID, len(files))
					engine.SetPlaylist(zoneID, files)
				})
				if err != nil {
					return fmt.Errorf("watcher init for zone %s: %w", zoneID, err)
				}

				engine.SetPlaylist(zoneID, w.Files())

				go func() {
					if err := w.Start(); err != nil {
						log.Printf("[main] watcher error for zone %s: %v", zoneID, err)
					}
				}()
				watchers = append(watchers, w)
			}
			defer func() {
				for _, w := range watchers {
					w.Stop()
				}
			}()

			// --- API Client (heartbeats) ---
			apiClient, err := api.NewClient(configPath, version)
			if err != nil {
				log.Printf("[main] api client warning: %v", err)
			}
			if apiClient != nil {
				go apiClient.StartHeartbeat()
				defer apiClient.Stop()
			}

			// --- Graceful Shutdown ---
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := engine.Play()

			select {
			case sig := <-sigCh:
				log.Printf("[main] received signal: %v — shutting down", sig)
				engine.Stop()
			case err := <-errCh:
				if err != nil {
					log.Printf("[main] zone error: %v", err)
				}
			}

			log.Println("[main] shutdown complete")
			return nil
		},
	}

	cmd.Flags().StringVarP(&playlistDir, "playlist", "p", defaultPlaylistDir(), "Path to the media playlist directory")
	cmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "Path to config.json identity file")
	cmd.Flags().StringVarP(&templatePath, "template", "t", "", "Path to a template JSON file (default: fullscreen)")
	cmd.Flags().IntVar(&screenW, "screen-width", 1920, "Screen width in pixels (for zone positioning)")
	cmd.Flags().IntVar(&screenH, "screen-height", 1080, "Screen height in pixels (for zone positioning)")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("n-compasstv %s\nBuilt: %s\n", version, buildTime)
		},
	}
}

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run a system health check",
		Run: func(cmd *cobra.Command, args []string) {
			log.SetFlags(log.LstdFlags)
			status := system.RunHealthCheck()
			fmt.Printf("CPU Temperature : %.1f°C\n", status.CPUTempC)
			fmt.Printf("Disk Usage      : %.1f%%\n", status.DiskUsedPct)
			fmt.Printf("Disk Free       : %d MB\n", status.DiskFreeBytes/1024/1024)
			fmt.Printf("Throttled       : %v\n", status.Throttled)
		},
	}
}

func defaultPlaylistDir() string {
	if runtime.GOOS == "windows" {
		exe, _ := os.Executable()
		return filepath.Join(filepath.Dir(exe), "playlist")
	}
	return "/playlist"
}

func defaultConfigPath() string {
	if runtime.GOOS == "windows" {
		exe, _ := os.Executable()
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	return "/etc/n-compasstv/config.json"
}
