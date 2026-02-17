#!/usr/bin/env bash
# =============================================================================
# n-compasstv: Remote Install Script
#
# Install on any Raspberry Pi 5 with a single command:
#
#   curl -sSL https://raw.githubusercontent.com/user/n-compasstv/main/scripts/install.sh | sudo bash
#
# This script:
#   1. Detects the Pi architecture
#   2. Downloads the latest .deb from GitHub Releases
#   3. Installs it via apt (handles dependencies automatically)
#   4. Configures GPU memory and KMS for 4K playback
# =============================================================================

set -euo pipefail

# ---- CONFIGURATION ----
# Change this to your GitHub org/user and repo name after pushing.
GITHUB_REPO="ncompasstv/n-compasstv"
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
    fail "This script must be run as root. Use: curl ... | sudo bash"
fi

ARCH=$(dpkg --print-architecture 2>/dev/null || uname -m)
case "$ARCH" in
    arm64|aarch64) DEB_ARCH="arm64" ;;
    amd64|x86_64)  DEB_ARCH="amd64" ;;
    *)             fail "Unsupported architecture: $ARCH" ;;
esac

log "=== Installing ${APP} ==="
log "Architecture: ${DEB_ARCH}"
echo ""

# --- Detect latest release ---
log "Fetching latest release from GitHub..."

if ! command -v curl &>/dev/null; then
    apt-get update -qq && apt-get install -y curl
fi

LATEST_TAG=$(curl -sSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)

if [[ -z "$LATEST_TAG" ]]; then
    fail "Could not find latest release. Check that ${GITHUB_REPO} has published releases."
fi

VERSION="${LATEST_TAG#v}"
DEB_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_TAG}/${APP}_${VERSION}_${DEB_ARCH}.deb"

log "Latest version: ${VERSION}"
log "Download URL: ${DEB_URL}"

# --- Download ---
TMP_DEB="/tmp/${APP}_${VERSION}_${DEB_ARCH}.deb"

log "Downloading .deb package..."
curl -sSL -o "$TMP_DEB" "$DEB_URL" || fail "Download failed. Check the URL: ${DEB_URL}"

# --- Install VLC dependencies first ---
log "Installing VLC dependencies..."
apt-get update -qq
apt-get install -y --no-install-recommends \
    libvlc-dev \
    vlc-plugin-base \
    vlc-plugin-video-output

# --- Install the .deb ---
log "Installing ${APP} v${VERSION}..."
apt install -y "$TMP_DEB"
rm -f "$TMP_DEB"

# --- GPU / Boot config for 4K ---
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
echo ""
log "============================================"
log "  ${APP} v${VERSION} installed!"
log "============================================"
echo ""
echo "  Quick start:"
echo "    sudo cp *.mp4 /playlist/"
echo "    n-compasstv run"
echo ""
echo "  Or as a service:"
echo "    sudo systemctl start n-compasstv"
echo ""
echo "  Commands:"
echo "    n-compasstv run         Start player"
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
