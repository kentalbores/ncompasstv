#!/usr/bin/env bash
# =============================================================================
# n-compasstv: Raspberry Pi 5 Setup Script
#
# This script builds the .deb on the Pi and installs it via apt.
# After running this, you can use: n-compasstv run
#
# Usage:
#   chmod +x scripts/setup-pi.sh
#   sudo ./scripts/setup-pi.sh
# =============================================================================

set -euo pipefail

APP="n-compasstv"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[${APP}]${NC} $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }
fail() { echo -e "${RED}[error]${NC} $1"; exit 1; }

# --- Pre-checks ---
if [[ $EUID -ne 0 ]]; then
    fail "Run with sudo: sudo ./scripts/setup-pi.sh"
fi

ARCH=$(uname -m)
log "=== ${APP} Setup for Raspberry Pi 5 ==="
log "Architecture: $ARCH"
echo ""

# --- Step 1: System Dependencies ---
log "Step 1/5: Installing system dependencies..."
apt-get update -qq
apt-get install -y --no-install-recommends \
    libvlc-dev \
    vlc-plugin-base \
    vlc-plugin-video-output \
    pkg-config \
    wget

# --- Step 2: Install Go ---
GO_VERSION="1.22.5"
if command -v go &>/dev/null; then
    log "Step 2/5: Go already installed ($(go version | awk '{print $3}')), skipping."
else
    log "Step 2/5: Installing Go ${GO_VERSION}..."

    if [[ "$ARCH" == "aarch64" ]]; then
        GO_ARCH="arm64"
    elif [[ "$ARCH" == "x86_64" ]]; then
        GO_ARCH="amd64"
    else
        GO_ARCH="arm64"
    fi

    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
    export PATH=$PATH:/usr/local/go/bin
    log "Go $(go version | awk '{print $3}') installed."
fi

export PATH=$PATH:/usr/local/go/bin

# --- Step 3: Build ---
log "Step 3/5: Building ${APP}..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

REAL_USER=$(logname 2>/dev/null || echo "${SUDO_USER:-root}")

sudo -u "$REAL_USER" bash -c "
    export PATH=\$PATH:/usr/local/go/bin
    export CGO_ENABLED=1
    cd $PROJECT_DIR
    go mod tidy
    mkdir -p build
    go build -trimpath \
        -ldflags '-s -w -X main.version=0.1.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
        -o build/${APP} ./cmd/player
"

log "Binary built: $PROJECT_DIR/build/${APP}"

# --- Step 4: Build .deb and install ---
log "Step 4/5: Building .deb package..."
bash scripts/package.sh "0.1.0" "build"

DEB_FILE="build/deb/${APP}_0.1.0_arm64.deb"

log "Step 5/5: Installing via apt..."
apt install -y "./${DEB_FILE}"

# --- Step 5: Boot config for 4K ---
BOOT_CONFIG="/boot/firmware/config.txt"
[[ ! -f "$BOOT_CONFIG" ]] && BOOT_CONFIG="/boot/config.txt"

NEEDS_REBOOT=false

if [[ -f "$BOOT_CONFIG" ]]; then
    if ! grep -q "^gpu_mem=256" "$BOOT_CONFIG" 2>/dev/null; then
        if grep -q "^gpu_mem=" "$BOOT_CONFIG"; then
            sed -i "s/^gpu_mem=.*/gpu_mem=256/" "$BOOT_CONFIG"
        else
            echo "gpu_mem=256" >> "$BOOT_CONFIG"
        fi
        NEEDS_REBOOT=true
    fi

    if ! grep -q "^dtoverlay=vc4-kms-v3d" "$BOOT_CONFIG" 2>/dev/null; then
        echo "dtoverlay=vc4-kms-v3d" >> "$BOOT_CONFIG"
        NEEDS_REBOOT=true
    fi
fi

# Disable screen blanking
if [[ -f /boot/cmdline.txt ]] && ! grep -q "consoleblank=0" /boot/cmdline.txt; then
    sed -i 's/$/ consoleblank=0/' /boot/cmdline.txt
    NEEDS_REBOOT=true
fi

# --- Done ---
echo ""
log "============================================"
log "  ${APP} installed successfully!"
log "============================================"
echo ""
echo "  Usage:"
echo "    n-compasstv run                        # Start manually"
echo "    n-compasstv version                    # Show version"
echo "    n-compasstv check                      # Health check"
echo ""
echo "  Service:"
echo "    sudo systemctl start n-compasstv       # Start"
echo "    sudo systemctl stop n-compasstv        # Stop"
echo "    sudo systemctl status n-compasstv      # Status"
echo "    journalctl -u n-compasstv -f           # Logs"
echo ""
echo "  Content:"
echo "    Copy videos/images to /playlist/"
echo ""
echo "  Config:"
echo "    /etc/n-compasstv/config.json"
echo ""

if [[ "$NEEDS_REBOOT" == true ]]; then
    warn "Boot config was changed. REBOOT required: sudo reboot"
fi
