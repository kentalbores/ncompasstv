ROLE

You are an expert Systems Engineer and Golang Developer specializing in High-Performance Embedded Systems and the Raspberry Pi 5.

TASK

Develop "Player-Native": A hardware-accelerated 4K video player for RPi5 using Go and libVLC, replacing a legacy Node.js/Chromium stack.

ARCHITECTURE & TECH STACK

Language: Go (Golang) 1.22+.

Video Engine: github.com/adrg/libvlc-go/v3.

OS Target: Raspberry Pi OS 64-bit (Debian Bookworm).

Rendering: DRM/KMS (Direct Rendering Manager) to bypass the desktop compositor for 4K@60fps.

Folder Monitoring: github.com/fsnotify/fsnotify.

PROJECT STRUCTURE (STRICT ADHERENCE REQUIRED)

/cmd/player/main.go          -> Entry point. Use 'cobra' for CLI commands (run, version, check).
/internal/vlc/engine.go      -> libVLC Wrapper. Must handle CGO bindings and hardware flags.
/internal/playlist/watcher.go -> fsnotify logic. Watches '/playlist' folder. Updates queue on file events.
/internal/api/client.go      -> Port logic from legacy app.js. Handle heartbeats and config.json (ID/Key).
/internal/system/utils.go    -> Disk monitoring, resolution setting (fbset), and health checks.
/deploy/player.service       -> Systemd unit file for auto-start.
/scripts/package.sh          -> Script to build the .deb package.

HARDWARE OPTIMIZATION (CRITICAL)

Initialize libVLC with these specific flags for RPi5:

--vout=mmal_vout

--hwdec=mmal

--fullscreen

--no-osd

--dbus (disabled)

--no-video-title-show

IMPLEMENTATION STEPS

Initialize the Go module and folder structure.

Implement the VLC engine first to ensure 4K playback works.

Implement the File Watcher to loop through the '/playlist' folder alphabetically.

Implement the API client to read the existing config.json format.

Create a Makefile to handle cross-compilation and .deb packaging.

Generate a Multi-stage Dockerfile that installs libvlc-dev in the build stage.

DEPLOYMENT GOAL

I must be able to run:

'make build' to get a binary.

'make deb' to get a .deb file.

'sudo apt install ./player.deb' to install the player as a system service.

'player run' to manually start the engine.

Please begin by generating the directory structure and the core /internal/vlc/engine.go file.