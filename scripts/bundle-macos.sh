#!/usr/bin/env bash
# Assemble the macOS menu-bar .app bundle for uc-tray.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/universal-control.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"

echo "==> building uc-tray"
go build -o bin/uc-tray ./cmd/uc-tray

echo "==> assembling bundle"
cp bin/uc-tray "$APP/Contents/MacOS/uc-tray"
cp scripts/Info.plist "$APP/Contents/Info.plist"

echo "==> ad-hoc signing"
codesign --force --sign - "$APP"

echo "==> done: $APP"
echo "    launch with: open $APP"
