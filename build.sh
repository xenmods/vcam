#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Build webapp
echo "[VCam] Building webapp..."
cd "$REPO_ROOT/webapp" && npm run build

# Copy Vite output to go2rtc-webapp
VITE_OUT="$REPO_ROOT/mod/src/main/resources/assets/vcam/webapp"
GO2RTC_WEB="$REPO_ROOT/go2rtc-webapp"
rm -rf "$GO2RTC_WEB"
mkdir -p "$GO2RTC_WEB"
cp -r "$VITE_OUT/"* "$GO2RTC_WEB/"
echo "[VCam] Updated go2rtc-webapp/"

# Copy webapp files into app/webapp/ for embedding
WEBAPP_DST="$SCRIPT_DIR/webapp"
rm -rf "$WEBAPP_DST"
mkdir -p "$WEBAPP_DST"
cp -r "$GO2RTC_WEB/"* "$WEBAPP_DST/"
echo "[VCam] Copied webapp to app/webapp/"

# Build Go binary
cd "$SCRIPT_DIR"
go build -o vcam -ldflags="-s -w" .
echo "[VCam] Build successful: $SCRIPT_DIR/vcam"
