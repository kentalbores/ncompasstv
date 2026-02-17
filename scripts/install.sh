#!/usr/bin/env bash
# =============================================================================
# n-compasstv: Remote Install Script
#
# Install on any Raspberry Pi 5 with a single command:
#
#   curl -sSL https://raw.githubusercontent.com/kentalbores/ncompasstv/main/scripts/install.sh | sudo bash
#
# For quick updates after first install:
#
#   curl -sSL https://raw.githubusercontent.com/kentalbores/ncompasstv/main/scripts/update.sh | sudo bash
#
# This script:
#   1. Installs VLC + Go
#   2. Clones and builds the binary (pure Go, no CGO)
#   3. Creates and installs the .deb package
#   4. Installs all template files
#   5. Configures GPU/KMS for 4K playback
# =============================================================================

set -euo pipefail

GITHUB_REPO="kentalbores/ncompasstv"
APP="n-compasstv"
GO_VERSION="1.22.5"
SRC_DIR="/opt/n-compasstv"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[${APP}]${NC} $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }
fail() { echo -e "${RED}[error]${NC} $1"; exit 1; }

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
apt-get update -qq || true
apt-get install -y --no-install-recommends \
    git \
    wget \
    vlc \
    vlc-plugin-base \
    vlc-plugin-video-output \
    xdotool \
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
rm -rf "${SRC_DIR}"
git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "${SRC_DIR}"
cd "${SRC_DIR}"

# --- Step 4: Build the binary (pure Go, no CGO) ---
log "Step 4/6: Building ${APP}..."

GO_BIN="/usr/local/go/bin/go"
if [[ ! -x "$GO_BIN" ]]; then
    GO_BIN=$(which go)
fi

VERSION=$(git describe --tags --always 2>/dev/null || echo "")
VERSION="${VERSION#v}"
if [[ -z "$VERSION" ]] || ! [[ "$VERSION" =~ ^[0-9] ]]; then
    VERSION="0.1.0"
fi
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

log "Using Go: $GO_BIN ($(${GO_BIN} version))"

GOPATH=/tmp/go-path GOMODCACHE=/tmp/go-path/pkg/mod \
    ${GO_BIN} mod tidy

mkdir -p build
GOPATH=/tmp/go-path GOMODCACHE=/tmp/go-path/pkg/mod \
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

# Install templates.
mkdir -p /etc/n-compasstv/templates
cp -f templates/*.json /etc/n-compasstv/templates/ 2>/dev/null || true

# Install the update script.
cp -f scripts/update.sh /usr/local/bin/n-compasstv-update
chmod +x /usr/local/bin/n-compasstv-update

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

# --- Done ---
rm -rf /tmp/go-path

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
echo "    n-compasstv run -t /etc/n-compasstv/templates/nct-standard.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/main-with-footer.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/2-1-zone.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/1-to-1-zone.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/standard-v2.json"
echo "    n-compasstv run -t /etc/n-compasstv/templates/l-shape.json"
echo ""
echo "  Fast updates (after first install):"
echo "    sudo n-compasstv-update"
echo ""
echo "  Or cross-compile from your PC:"
echo "    GOOS=linux GOARCH=arm64 go build -trimpath -o n-compasstv ./cmd/player"
echo "    scp n-compasstv pi@<IP>:/usr/local/bin/"
echo ""
echo "  Service:"
echo "    sudo systemctl start n-compasstv"
echo "    sudo systemctl status n-compasstv"
echo "    journalctl -u n-compasstv -f"
echo ""

if [[ "$NEEDS_REBOOT" == true ]]; then
    warn "Boot config changed for 4K. Reboot required: sudo reboot"
fi
