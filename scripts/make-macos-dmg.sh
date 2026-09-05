#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/EdgeHop.app"
DMG="dist/EdgeHop-macos26-arm64.dmg"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

[ -d "$APP" ] || { echo "missing app bundle: run make bundle first" >&2; exit 1; }

echo "==> staging DMG"
cp -R "$APP" "$STAGE/EdgeHop.app"
ln -s /Applications "$STAGE/Applications"

rm -f "$DMG"
echo "==> creating DMG"
hdiutil create \
  -volname "EdgeHop" \
  -srcfolder "$STAGE" \
  -format UDZO \
  -ov \
  "$DMG"

codesign --verify --deep --strict --verbose=2 "$APP"
hdiutil verify "$DMG"
shasum -a 256 "$DMG" > "$DMG.sha256"

echo "==> done: $DMG"
