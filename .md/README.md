# n-compasstv

Hardware-accelerated 4K digital signage player for Raspberry Pi 5, replacing the legacy Node.js/Chromium stack.

Built with **Go + libVLC** for zero-overhead playback via DRM/KMS.

---

## Install on Raspberry Pi 5

### Quick Setup (recommended)

Copy the project to the Pi, then run:

```bash
chmod +x scripts/setup-pi.sh
sudo ./scripts/setup-pi.sh
```

This installs everything. After it finishes:

```bash
# Copy your content
sudo cp *.mp4 /playlist/

# Start the player
sudo systemctl start n-compasstv
```

### What the setup script does

1. Installs `libvlc-dev`, VLC plugins, `pkg-config`
2. Installs Go 1.22 for arm64
3. Builds the binary natively with CGO + MMAL hardware acceleration
4. Creates a `.deb` package and installs it via `apt`
5. Enables the systemd service (auto-start on boot)
6. Configures GPU memory (256MB) and KMS/DRM overlay

### Manual install via .deb

If you already have the `.deb` file:

```bash
sudo apt install ./n-compasstv_0.1.0_arm64.deb
```

---

## Usage

```bash
n-compasstv run                        # Start player (foreground)
n-compasstv run --playlist /my/videos  # Custom playlist directory
n-compasstv run --template layout.json # Multi-zone template
n-compasstv version                    # Print version
n-compasstv check                      # System health check
```

### As a system service

```bash
sudo systemctl start n-compasstv      # Start
sudo systemctl stop n-compasstv       # Stop
sudo systemctl status n-compasstv     # Status
sudo systemctl restart n-compasstv    # Restart
journalctl -u n-compasstv -f          # Live logs
```

---

## Architecture

```
cmd/player/main.go              CLI entry point (cobra: run, version, check)
internal/
  vlc/
    engine.go                   Zone-aware playback coordinator
    engine_prod.go              Production: CGO/libVLC + MMAL (linux/arm64)
    engine_dev.go               Development: VLC subprocess (Windows/macOS/x86)
  playlist/
    watcher.go                  Real-time folder monitoring (fsnotify)
    watcher_test.go             Unit tests
  media/
    media.go                    Media type detection (video vs image)
  template/
    template.go                 Zone layout system (JSON templates)
  api/
    client.go                   Heartbeat + config.json identity
  system/
    utils.go                    Disk, thermal, resolution, health checks
templates/
  fullscreen.json               Single zone, full screen (default)
  main-with-footer.json         Main area + footer strip
  l-shape.json                  Main + sidebar + footer
deploy/
  n-compasstv.service           Systemd unit (auto-restart, hardened)
scripts/
  setup-pi.sh                   One-command Pi setup
  package.sh                    Debian .deb packaging
```

---

## Playback Engine

### Gapless Transitions

Videos transition seamlessly with zero black frames between clips.

| Platform | Strategy |
|----------|----------|
| Production (RPi5) | libVLC `ListPlayer` with `Loop` mode — hardware-accelerated gapless |
| Development | Single VLC process with full playlist + `--loop` — native gapless |

### Supported Media

**Video**: `.mp4`, `.mkv`, `.avi`, `.mov`, `.webm`, `.ts`, `.m4v`, `.hevc`, `.flv`, `.wmv`

**Image**: `.jpg`, `.jpeg`, `.png`, `.bmp`, `.gif`, `.webp`, `.tiff`, `.svg` (displayed for 10 seconds)

Videos and images can be mixed. They play in alphabetical filename order.

### Performance Optimizations

```
Hardware Decoding
  Production:  MMAL decoder + MMAL video output (RPi5 GPU)
  Development: DXVA2/D3D11VA auto-detection (Windows GPU)

Buffering
  --file-caching=5000       5 second file read-ahead buffer
  --network-caching=3000    3 second network stream buffer

Quality Preservation
  --no-drop-late-frames     Never drop frames
  --no-skip-frames          Never skip B-frames
  --avcodec-skiploopfilter=0  Keep deblocking filter active
```

---

## Template System

Templates divide the screen into **zones**. Each zone has its own playlist directory, watcher, and engine.

### Built-in Templates

**fullscreen** (default)
```
+------------------------------------------+
|                  main                    |
|               (100x100%)                 |
+------------------------------------------+
```

**main-with-footer**
```
+------------------------------------------+
|                  main                    |
|               (100x85%)                  |
+------------------------------------------+
|              footer (100x15%)            |
+------------------------------------------+
```

**l-shape**
```
+-------------------------------+----------+
|            main               | sidebar  |
|          (75x85%)             | (25x100%)|
+-------------------------------+          |
|        footer (75x15%)        |          |
+-------------------------------+----------+
```

### Custom Templates

Create a JSON file:

```json
{
  "name": "my-layout",
  "zones": [
    {
      "id": "main",
      "x": 0, "y": 0, "width": 70, "height": 100,
      "playlist_dir": "/playlist/main"
    },
    {
      "id": "sidebar",
      "x": 70, "y": 0, "width": 30, "height": 100,
      "playlist_dir": "/playlist/ads"
    }
  ]
}
```

Run with: `n-compasstv run --template my-layout.json`

---

## Playlist Management

Each zone watches its directory in real time:

1. **Add**: Copy files into the playlist directory
2. **Remove**: Delete files
3. **Reorder**: Prefix filenames with numbers (`01_intro.mp4`, `02_main.mp4`)

Changes detected instantly — no restart required.

---

## Configuration

Config file: `/etc/n-compasstv/config.json`

```json
{
  "id": "player-001",
  "key": "auth-secret-key",
  "name": "Lobby Display",
  "endpoint": "https://api.example.com",
  "heartbeat_interval_sec": 60
}
```

Heartbeats POST to `{endpoint}/heartbeat`. Runs standalone if no config exists.

---

## CLI Reference

```
n-compasstv run [flags]
  -p, --playlist string      Playlist directory (default: /playlist)
  -c, --config string        Config file (default: /etc/n-compasstv/config.json)
  -t, --template string      Template JSON (default: fullscreen)
      --screen-width int     Screen width in px (default: 1920)
      --screen-height int    Screen height in px (default: 1080)

n-compasstv version          Print version and build time
n-compasstv check            System health (CPU temp, disk, throttle)
```
