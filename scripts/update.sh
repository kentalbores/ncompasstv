#!/usr/bin/env bash
# =============================================================================
# n-compasstv: Quick Update Script
#
# Run on the Pi to pull the latest code and rebuild in seconds:
#
#   curl -sSL https://raw.githubusercontent.com/kentalbores/ncompasstv/main/scripts/update.sh | sudo bash
#
# Or after first install, just run:
#   sudo n-compasstv-update
# =============================================================================

set -euo pipefail

APP="n-compasstv"
GITHUB_REPO="kentalbores/ncompasstv"
SRC_DIR="/opt/n-compasstv"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log()  { echo -e "${GREEN}[${APP}]${NC} $1"; }
fail() { echo -e "${RED}[error]${NC} $1"; exit 1; }

if [[ $EUID -ne 0 ]]; then
    fail "Run with sudo"
fi

export PATH=$PATH:/usr/local/go/bin

# Ensure Go is installed.
if ! command -v go &>/dev/null; then
    fail "Go not installed. Run the full install first: curl -sSL https://raw.githubusercontent.com/kentalbores/ncompasstv/main/scripts/install.sh | sudo bash"
fi

# Clone or pull.
if [[ -d "$SRC_DIR/.git" ]]; then
    log "Pulling latest..."
    cd "$SRC_DIR"
    git fetch --all
    git reset --hard origin/main
else
    log "Cloning repo..."
    rm -rf "$SRC_DIR"
    git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$SRC_DIR"
    cd "$SRC_DIR"
fi

# Build (no CGO needed — pure Go).
log "Building..."
GOPATH=/tmp/go-path GOMODCACHE=/tmp/go-path/pkg/mod \
    go build -trimpath -o "/usr/local/bin/${APP}" ./cmd/player

# Copy templates.
mkdir -p /etc/n-compasstv/templates
cp -f templates/*.json /etc/n-compasstv/templates/ 2>/dev/null || true

# Restart service if running.
if systemctl is-active --quiet n-compasstv 2>/dev/null; then
    log "Restarting service..."
    systemctl restart n-compasstv
fi

VERSION=$(/usr/local/bin/n-compasstv version 2>/dev/null | head -1 || echo "unknown")
log "Updated! ${VERSION}"
log "Run: n-compasstv run"
