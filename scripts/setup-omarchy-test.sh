#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export UC_INSTALL_ROOT="$TMP/root"
export UC_SKIP_SYSTEM_COMMANDS=1
export HOME="$TMP/home"
export USER="testuser"
mkdir -p "$HOME/.config/hypr"
mkdir -p "$UC_INSTALL_ROOT/etc/systemd/system" "$UC_INSTALL_ROOT/usr/local/bin"
printf 'legacy service\n' > "$UC_INSTALL_ROOT/etc/systemd/system/uc-client.service"
printf 'legacy binary\n' > "$UC_INSTALL_ROOT/usr/local/bin/uc-client"
cat > "$HOME/.config/hypr/hyprland.conf" <<'EOF'
# --- universal-control: 虚拟指针去加速 ---
input-device {
    name = universal-control
    accel_profile = flat
    sensitivity = 0
}
EOF

BIN="$TMP/uc-client"
printf '#!/bin/sh\n' > "$BIN"
chmod +x "$BIN"
PAIRING_CODE="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"

printf '%s\n' "$PAIRING_CODE" | "$ROOT/scripts/setup-omarchy.sh" \
  -s 192.0.2.10 \
  -b "$BIN"

SERVICE="$UC_INSTALL_ROOT/etc/systemd/system/edgehop-client.service"
KEY="$HOME/.config/edgehop/pairing.key"
HYPR="$HOME/.config/hypr/hyprland.conf"

grep -Fq -- "-server 192.0.2.10:24800" "$SERVICE"
grep -Fq -- "-pairing-file $HOME/.config/edgehop/pairing.key" "$SERVICE"
if grep -Fq -- "-pairing-file %h/" "$SERVICE"; then
  echo "system service must not use manager-relative %h for the pairing key" >&2
  exit 1
fi
[ -x "$UC_INSTALL_ROOT/usr/local/bin/edgehop-client" ]
[ ! -e "$UC_INSTALL_ROOT/etc/systemd/system/uc-client.service" ]
[ ! -e "$UC_INSTALL_ROOT/usr/local/bin/uc-client" ]
[ "$(cat "$KEY")" = "$PAIRING_CODE" ]
if stat -f '%Lp' "$KEY" >/dev/null 2>&1; then
  KEY_MODE="$(stat -f '%Lp' "$KEY")"
else
  KEY_MODE="$(stat -c '%a' "$KEY")"
fi
[ "$KEY_MODE" = "600" ]
grep -Fq "device {" "$HYPR"
grep -Fq "name = edgehop" "$HYPR"
if grep -Fq "universal-control" "$HYPR"; then
  echo "legacy device name remains" >&2
  exit 1
fi
if grep -Fq "input-device {" "$HYPR"; then
  echo "invalid input-device block remains" >&2
  exit 1
fi

printf '%s\n' "$PAIRING_CODE" | "$ROOT/scripts/setup-omarchy.sh" \
  -s 192.0.2.20 \
  -b "$BIN"

grep -Fq -- "-server 192.0.2.20:24800" "$SERVICE"
if grep -Fq "192.0.2.10" "$SERVICE"; then
  echo "old server address survived reinstall" >&2
  exit 1
fi

echo "setup-omarchy tests passed"
