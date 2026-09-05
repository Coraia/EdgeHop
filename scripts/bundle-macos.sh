#!/usr/bin/env bash
# Assemble the EdgeHop macOS menu-bar app bundle.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/EdgeHop.app"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

echo "==> building EdgeHop"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=26.0 go build \
  -trimpath \
  -ldflags="-s -w" \
  -o bin/edgehop \
  ./cmd/edgehop

echo "==> assembling bundle"
cp bin/edgehop "$APP/Contents/MacOS/edgehop"
cp scripts/Info.plist "$APP/Contents/Info.plist"

ICONSET="$TMP/EdgeHop.iconset"
mkdir -p "$ICONSET"
for spec in \
  "16 icon_16x16.png" \
  "32 icon_16x16@2x.png" \
  "32 icon_32x32.png" \
  "64 icon_32x32@2x.png" \
  "128 icon_128x128.png" \
  "256 icon_128x128@2x.png" \
  "256 icon_256x256.png" \
  "512 icon_256x256@2x.png" \
  "512 icon_512x512.png" \
  "1024 icon_512x512@2x.png"; do
  read -r size name <<<"$spec"
  sips -z "$size" "$size" assets/edgehop-app-icon.png --out "$ICONSET/$name" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/EdgeHop.icns"

echo "==> ad-hoc signing"
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

MINOS="$(otool -l "$APP/Contents/MacOS/edgehop" | awk '/minos/{print $2; exit}')"
if [[ "$MINOS" != "26.0" ]]; then
  echo "unexpected deployment target: $MINOS (want 26.0)" >&2
  exit 1
fi
if ! file "$APP/Contents/MacOS/edgehop" | grep -q "arm64"; then
  echo "unexpected binary architecture (want arm64)" >&2
  exit 1
fi

echo "==> done: $APP"
echo "    launch with: open $APP"
