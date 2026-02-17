#!/usr/bin/env bash
# =============================================================================
# n-compasstv: Remote Install Script
#
# Install on any Raspberry Pi 5 with a single command:
#
#   curl -sSL https://raw.githubusercontent.com/kentalbores/ncompasstv/main/scripts/install.sh | sudo bash
#
# This script:
#   1. Clones the repo
#   2. Installs Go + libVLC dependencies
#   3. Builds the binary natively on the Pi (CGO + MMAL hardware accel)
#   4. Creates and installs the .deb package via apt
#   5. Configures GPU memory and KMS for 4K playback
# =============================================================================

set -euo pipefail

GITHUB_REPO="kentalbores/ncompasstv"
APP="n-compasstv"
GO_VERSION="1.22.5"
INSTALL_DIR="/tmp/n-compasstv-install"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[${APP}]${NC} $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }
fail() { echo -e "${RED}[error]${NC} $1"; exit 1; }

# --- Pre-checks ---
if [[ $EUID -ne 0 ]]; then
    fail "Run with sudo: curl -sSL ... | sudo bash"
fi

ARCH=$(uname -m)
case "$ARCH" in
    aarch64) GO_ARCH="arm64"; DEB_ARCH="arm64" ;;
    x86_64)  GO_ARCH="amd64"; DEB_ARCH="amd64" ;;
    *)       fail "Unsupported architecture: $ARCH" ;;
esac

log "=== Installing ${APP} ==="
log "Architecture: ${ARCH} (${DEB_ARCH})"
echo ""

# --- Step 1: System dependencies ---
log "Step 1/6: Installing system dependencies..."
apt-get update -qq
apt-get install -y --no-install-recommends \
    git \
    wget \
    libvlc-dev \
    vlc-plugin-base \
    vlc-plugin-video-output \
    pkg-config \
    dpkg-dev

# --- Step 2: Install Go ---
if command -v go &>/dev/null; then
    log "Step 2/6: Go already installed ($(go version | awk '{print $3}')), skipping."
else
    log "Step 2/6: Installing Go ${GO_VERSION} for ${GO_ARCH}..."
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
fi
export PATH=$PATH:/usr/local/go/bin

# --- Step 3: Clone the repo ---
log "Step 3/6: Cloning ${GITHUB_REPO}..."
rm -rf "${INSTALL_DIR}"
git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "${INSTALL_DIR}"
cd "${INSTALL_DIR}"

# --- Step 4: Build the binary ---
log "Step 4/6: Building ${APP} (native CGO + MMAL)..."

GO_BIN="/usr/local/go/bin/go"
if [[ ! -x "$GO_BIN" ]]; then
    GO_BIN=$(which go)
fi

VERSION=$(git describe --tags --always 2>/dev/null || echo "")
VERSION="${VERSION#v}"
# Debian requires version to start with a digit.
if [[ -z "$VERSION" ]] || ! [[ "$VERSION" =~ ^[0-9] ]]; then
    VERSION="0.1.0"
fi
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

log "Using Go: $GO_BIN ($(${GO_BIN} version))"
log "Working directory: $(pwd)"
log "go.mod module: $(head -1 go.mod)"

GO111MODULE=on CGO_ENABLED=1 GOPATH=/tmp/go-path GOMODCACHE=/tmp/go-path/pkg/mod \
    ${GO_BIN} mod tidy

mkdir -p build
GO111MODULE=on CGO_ENABLED=1 GOPATH=/tmp/go-path GOMODCACHE=/tmp/go-path/pkg/mod \
    ${GO_BIN} build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o "build/${APP}" ./cmd/player

log "Binary built successfully."

# --- Step 5: Build .deb and install ---
log "Step 5/6: Building .deb package..."
bash scripts/package.sh "${VERSION}" build

DEB_FILE="build/deb/${APP}_${VERSION}_${DEB_ARCH}.deb"

if [[ ! -f "${DEB_FILE}" ]]; then
    fail ".deb not found at ${DEB_FILE}"
fi

log "Installing .deb package..."
dpkg -i --force-overwrite "./${DEB_FILE}"
apt-get install -f -y

# --- Step 6: GPU / Boot config for 4K ---
log "Step 6/6: Configuring boot for 4K..."

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

if [[ -f /boot/cmdline.txt ]] && ! grep -q "consoleblank=0" /boot/cmdline.txt; then
    sed -i 's/$/ consoleblank=0/' /boot/cmdline.txt
    NEEDS_REBOOT=true
fi

# --- Cleanup ---
cd /
rm -rf "${INSTALL_DIR}"
rm -rf /tmp/go-path

# --- Done ---
echo ""
log "============================================"
log "  ${APP} v${VERSION} installed!"
log "============================================"
echo ""
echo "  Quick start:"
echo "    sudo cp *.mp4 /playlist/"
echo "    n-compasstv run"
echo ""
echo "  Templates (multi-zone layouts):"
echo "    n-compasstv run -t /etc/n-compasstv/templates/fullscreen.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/main-with-footer.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/l-shape.json"
echo ""
echo "  As a service (starts on boot):"
echo "    sudo systemctl start n-compasstv"
echo ""
echo "  Commands:"
echo "    n-compasstv run         Start player (fullscreen)"
echo "    n-compasstv version     Show version"
echo "    n-compasstv check       Health check"
echo ""
echo "  Service:"
echo "    sudo systemctl status n-compasstv"
echo "    journalctl -u n-compasstv -f"
echo ""

if [[ "$NEEDS_REBOOT" == true ]]; then
    warn "Boot config changed for 4K. Reboot required: sudo reboot"
fi
