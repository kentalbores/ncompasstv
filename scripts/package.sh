#!/usr/bin/env bash
# package.sh — Builds a Debian .deb package for n-compasstv.
# Usage: ./scripts/package.sh <version> <build_dir>

set -euo pipefail

VERSION="${1:-0.1.0}"
BUILD_DIR="${2:-build}"
PKG_NAME="n-compasstv"
ARCH="arm64"

DEB_ROOT="${BUILD_DIR}/deb/${PKG_NAME}_${VERSION}_${ARCH}"

echo "==> Packaging ${PKG_NAME} v${VERSION} (${ARCH})"

# Clean previous build
rm -rf "${DEB_ROOT}"

# Create directory structure
mkdir -p "${DEB_ROOT}/DEBIAN"
mkdir -p "${DEB_ROOT}/usr/local/bin"
mkdir -p "${DEB_ROOT}/etc/systemd/system"
mkdir -p "${DEB_ROOT}/etc/n-compasstv"
mkdir -p "${DEB_ROOT}/var/log/n-compasstv"
mkdir -p "${DEB_ROOT}/playlist"

# Copy the binary
cp "${BUILD_DIR}/n-compasstv" "${DEB_ROOT}/usr/local/bin/n-compasstv"
chmod 755 "${DEB_ROOT}/usr/local/bin/n-compasstv"

# Copy the systemd unit
cp deploy/n-compasstv.service "${DEB_ROOT}/etc/systemd/system/n-compasstv.service"

# Generate a default config.json if one doesn't exist
cat > "${DEB_ROOT}/etc/n-compasstv/config.json" <<CONFIGEOF
{
  "id": "",
  "key": "",
  "name": "n-compasstv",
  "endpoint": "",
  "heartbeat_interval_sec": 60
}
CONFIGEOF

# Generate the DEBIAN control file
cat > "${DEB_ROOT}/DEBIAN/control" <<CONTROLEOF
Package: ${PKG_NAME}
Version: ${VERSION}
Section: video
Priority: optional
Architecture: ${ARCH}
Depends: libvlc-dev, vlc-plugin-base
Maintainer: nCompass Team <team@ncompass.tv>
Description: n-compasstv — 4K hardware-accelerated digital signage player
 A high-performance video player built with Go and libVLC,
 optimized for Raspberry Pi 5 with DRM/KMS rendering.
 Supports multi-zone templates, gapless playback, and images.
CONTROLEOF

# Post-install script: enable and start the service
cat > "${DEB_ROOT}/DEBIAN/postinst" <<'POSTEOF'
#!/bin/bash
set -e

# Reload systemd
systemctl daemon-reload

# Enable the service to start on boot
systemctl enable n-compasstv.service

# Create the playlist directory if missing
mkdir -p /playlist

# Configure GPU memory for 4K decoding
BOOT_CONFIG="/boot/firmware/config.txt"
[ ! -f "$BOOT_CONFIG" ] && BOOT_CONFIG="/boot/config.txt"

if [ -f "$BOOT_CONFIG" ]; then
    if ! grep -q "^gpu_mem=" "$BOOT_CONFIG" 2>/dev/null; then
        echo "gpu_mem=256" >> "$BOOT_CONFIG"
    fi
    if ! grep -q "^dtoverlay=vc4-kms-v3d" "$BOOT_CONFIG" 2>/dev/null; then
        echo "dtoverlay=vc4-kms-v3d" >> "$BOOT_CONFIG"
    fi
fi

echo ""
echo "================================================"
echo "  n-compasstv installed successfully!"
echo ""
echo "  Start:   sudo systemctl start n-compasstv"
echo "  Status:  sudo systemctl status n-compasstv"
echo "  Logs:    journalctl -u n-compasstv -f"
echo "  Manual:  n-compasstv run"
echo ""
echo "  Content: Copy media to /playlist/"
echo "  Config:  /etc/n-compasstv/config.json"
echo "================================================"
echo ""
POSTEOF
chmod 755 "${DEB_ROOT}/DEBIAN/postinst"

# Config file marker — prevents overwriting user edits on upgrade
cat > "${DEB_ROOT}/DEBIAN/conffiles" <<'CONFEOF'
/etc/n-compasstv/config.json
CONFEOF

# Pre-remove script: stop the service before uninstall
cat > "${DEB_ROOT}/DEBIAN/prerm" <<'PRERMEOF'
#!/bin/bash
set -e
systemctl stop n-compasstv.service || true
systemctl disable n-compasstv.service || true
PRERMEOF
chmod 755 "${DEB_ROOT}/DEBIAN/prerm"

# Build the .deb
dpkg-deb --build --root-owner-group "${DEB_ROOT}"

DEB_FILE="${DEB_ROOT}.deb"
echo ""
echo "==> Package built: ${DEB_FILE}"
echo "==> Install with:  sudo apt install ./${DEB_FILE}"
