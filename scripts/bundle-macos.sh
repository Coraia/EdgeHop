#!/usr/bin/env bash
# Assemble the macOS menu-bar .app bundle for uc-tray.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/universal-control.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"

echo "==> building uc-tray"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=26.0 go build \
  -trimpath \
  -ldflags="-s -w" \
  -o bin/uc-tray \
  ./cmd/uc-tray

echo "==> assembling bundle"
cp bin/uc-tray "$APP/Contents/MacOS/uc-tray"
cp scripts/Info.plist "$APP/Contents/Info.plist"

echo "==> ad-hoc signing"
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

MINOS="$(otool -l "$APP/Contents/MacOS/uc-tray" | awk '/minos/{print $2; exit}')"
if [[ "$MINOS" != "26.0" ]]; then
  echo "unexpected deployment target: $MINOS (want 26.0)" >&2
  exit 1
fi
if ! file "$APP/Contents/MacOS/uc-tray" | grep -q "arm64"; then
  echo "unexpected binary architecture (want arm64)" >&2
  exit 1
fi

echo "==> done: $APP"
echo "    launch with: open $APP"
