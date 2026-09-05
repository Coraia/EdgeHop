#!/usr/bin/env bash
# 在 Omarchy（Arch + Hyprland / Wayland）上一键安装并自启 EdgeHop 客户端。
#
# 用法：
#   ./setup-omarchy.sh -s 192.168.1.10
#   ./setup-omarchy.sh -s 192.168.1.10 -b ./edgehop-client-linux-amd64
# 脚本会静默提示粘贴配对码，避免密钥进入 shell 历史和进程参数。
#
# 做什么：
#   1) pacman 安装依赖   wl-clipboard hyprland-utils
#   2) 配置 /dev/uinput 访问（input 组 + udev 规则）
#   3) 安装 edgehop-client 到 /usr/local/bin
#   4) 写 systemd 服务（开机自启，覆盖登录界面）+ Hyprland 虚拟指针去加速
#
# 说明：edgehop-client 已支持在缺失会话环境变量时自动探测 Hyprland/Wayland
# socket，因此用 systemd 服务（multi-user.target）从开机一路管到登录会话，
# 登录界面（SDDM greeter）阶段即可被 Mac 键鼠控制，无需登录后再拉起。
set -euo pipefail

SERVER=""
BIN=""
PAIRING_CODE=""
INSTALL_ROOT="${UC_INSTALL_ROOT:-}"
SKIP_SYSTEM_COMMANDS="${UC_SKIP_SYSTEM_COMMANDS:-0}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)   BIN_DEFAULT="./edgehop-client-linux-amd64" ;;
  aarch64|arm64) BIN_DEFAULT="./edgehop-client-linux-arm64" ;;
  *) echo "无法识别的架构: $ARCH（请用 -b 指定客户端二进制）" >&2; exit 1 ;;
esac

usage() { echo "用法: $0 -s <Mac IP> [-b <客户端二进制>]"; exit 1; }

while getopts "s:b:h" opt; do
  case "$opt" in
    s) SERVER="$OPTARG" ;;
    b) BIN="$OPTARG" ;;
    *) usage ;;
  esac
done
[ -n "$SERVER" ] || usage
[ -n "$BIN" ] || BIN="$BIN_DEFAULT"
[ -f "$BIN" ] || { echo "找不到客户端二进制: $BIN" >&2; exit 1; }
[[ "$SERVER" =~ ^[A-Za-z0-9.-]+$ ]] || { echo "无效的 Mac 地址: $SERVER" >&2; exit 1; }

if [ -t 0 ]; then
  read -r -s -p "粘贴 Mac 菜单中复制的配对码: " PAIRING_CODE
  echo
else
  IFS= read -r PAIRING_CODE
fi
[[ "$PAIRING_CODE" =~ ^[A-Za-z0-9_-]{43}$ ]] || { echo "无效的配对码。" >&2; exit 1; }

if [ "$SKIP_SYSTEM_COMMANDS" != "1" ] && ! command -v pacman >/dev/null 2>&1; then
  echo "未找到 pacman，本脚本仅支持 Arch 系（Omarchy）。" >&2
  exit 1
fi

echo "==> 1/4 安装依赖 (wl-clipboard)"
if [ "$SKIP_SYSTEM_COMMANDS" != "1" ]; then
  sudo pacman -S --needed --noconfirm wl-clipboard || { echo "安装 wl-clipboard 失败，请检查网络/源。" >&2; exit 1; }
  # hyprctl 由 hyprland 包提供；hyprland-utils 仅在部分发行版存在，缺失不影响功能
  sudo pacman -S --needed --noconfirm hyprland-utils 2>/dev/null \
    || echo "    (hyprland-utils 不存在，hyprctl 已随 hyprland 提供，跳过)"
  command -v hyprctl >/dev/null 2>&1 || echo "    (警告: 未找到 hyprctl，边缘切回检测不可用)"
fi

echo "==> 2/4 配置 /dev/uinput 访问权限"
RULES="$INSTALL_ROOT/etc/udev/rules.d/99-edgehop.rules"
LEGACY_RULES="$INSTALL_ROOT/etc/udev/rules.d/99-universal-control.rules"
if [ "$SKIP_SYSTEM_COMMANDS" = "1" ]; then
  mkdir -p "$(dirname "$RULES")"
  echo 'KERNEL=="uinput", MODE="0660", GROUP="input", OPTIONS+="static_node=uinput"' > "$RULES"
  rm -f "$LEGACY_RULES"
else
  sudo usermod -aG input "$USER"
  echo 'KERNEL=="uinput", MODE="0660", GROUP="input", OPTIONS+="static_node=uinput"' | sudo tee "$RULES" >/dev/null
  sudo rm -f "$LEGACY_RULES"
  sudo udevadm control --reload-rules
  sudo udevadm trigger
fi

echo "==> 3/4 安装 edgehop-client 到 /usr/local/bin"
CLIENT_BIN="$INSTALL_ROOT/usr/local/bin/edgehop-client"
LEGACY_CLIENT_BIN="$INSTALL_ROOT/usr/local/bin/uc-client"
if [ "$SKIP_SYSTEM_COMMANDS" = "1" ]; then
  mkdir -p "$(dirname "$CLIENT_BIN")"
  install -m 755 "$BIN" "$CLIENT_BIN"
  rm -f "$LEGACY_CLIENT_BIN"
else
  sudo install -m 755 "$BIN" "$CLIENT_BIN"
  sudo rm -f "$LEGACY_CLIENT_BIN"
fi

PAIRING_DIR="$HOME/.config/edgehop"
PAIRING_FILE="$PAIRING_DIR/pairing.key"
install -d -m 700 "$PAIRING_DIR"
printf '%s\n' "$PAIRING_CODE" > "$PAIRING_FILE"
chmod 600 "$PAIRING_FILE"

echo "==> 4/4 配置 systemd 开机自启 + Hyprland 虚拟指针去加速"
# --- systemd 服务：开机即启动（multi-user.target），覆盖登录界面阶段 ---
SERVICE="$INSTALL_ROOT/etc/systemd/system/edgehop-client.service"
LEGACY_SERVICE="$INSTALL_ROOT/etc/systemd/system/uc-client.service"
SERVICE_CONTENT="[Unit]
Description=EdgeHop client (boot + login keyboard/mouse)
After=network.target

[Service]
Type=simple
User=$USER
ExecStart=/usr/local/bin/edgehop-client -server $SERVER:24800 -pairing-file $PAIRING_FILE -edge right
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target"

if [ "$SKIP_SYSTEM_COMMANDS" = "1" ]; then
  mkdir -p "$(dirname "$SERVICE")"
  rm -f "$LEGACY_SERVICE"
  printf '%s\n' "$SERVICE_CONTENT" > "$SERVICE"
else
  sudo systemctl disable --now uc-client.service >/dev/null 2>&1 || true
  sudo rm -f "$LEGACY_SERVICE"
  printf '%s\n' "$SERVICE_CONTENT" | sudo tee "$SERVICE" >/dev/null
  sudo systemctl daemon-reload
  sudo systemctl enable edgehop-client.service
  sudo systemctl restart edgehop-client.service
fi
echo "    已更新 ${SERVICE}。"

# --- Hyprland 虚拟指针去加速（无登录自启；启动由 systemd 接管） ---
HYPR_DIR="$HOME/.config/hypr"
mkdir -p "$HYPR_DIR"

# migrate_device_name FILE：就地重写旧 universal-control 设备名为 edgehop。
migrate_device_name() {
  local file="$1"
  grep -q "universal-control" "$file" 2>/dev/null || return 0
  local tmp="${file}.edgehop.tmp"
  sed 's/universal-control/edgehop/g' "$file" > "$tmp"
  mv "$tmp" "$file"
  echo "    已迁移旧 universal-control 设备配置: $file"
}

if [ -f "$HYPR_DIR/hyprland.lua" ]; then
  # --- Omarchy 风格 Lua 配置 ---
  INPUT="$HYPR_DIR/input.lua"
  migrate_device_name "$INPUT"
  if grep -q "edgehop" "$INPUT" 2>/dev/null; then
    echo "    input.lua 已包含 edgehop，跳过。"
  else
    cat >> "$INPUT" <<EOF

-- edgehop: 虚拟指针去加速（配合 hyprctl eval 可即时生效）
hl.device({
  name = "edgehop",
  accel_profile = "flat",
  sensitivity = 0,
})
EOF
    echo "    已写入 input.lua（虚拟指针去加速）。"
  fi
else
  # --- 标准 hyprland.conf（通用 Arch/Hyprland） ---
  HYPR="$HYPR_DIR/hyprland.conf"
  touch "$HYPR"
  migrate_device_name "$HYPR"
  if grep -q "input-device {" "$HYPR" && grep -q "edgehop" "$HYPR"; then
    TMP_HYPR="${HYPR}.edgehop.tmp"
    awk '
      /^# --- edgehop: 虚拟指针去加速 ---$/ { in_edgehop = 1 }
      in_edgehop && /^input-device[[:space:]]*\{/ { sub(/^input-device/, "device") }
      { print }
      in_edgehop && /^}/ { in_edgehop = 0 }
    ' "$HYPR" > "$TMP_HYPR"
    mv "$TMP_HYPR" "$HYPR"
    echo "    已迁移旧 input-device 配置。"
  fi
  if grep -q "edgehop" "$HYPR"; then
    echo "    hyprland.conf 已包含 edgehop，跳过。"
  else
    cat >> "$HYPR" <<EOF

# --- edgehop: 虚拟指针去加速 ---
device {
    name = edgehop
    accel_profile = flat
    sensitivity = 0
}
EOF
    echo "    已写入 device 去加速。"
  fi
fi

echo ""
echo "=================================================="
echo " 完成！已安装并配置 EdgeHop 客户端"
echo "   服务器地址 : $SERVER:24800"
echo "   客户端路径 : /usr/local/bin/edgehop-client"
echo "   配对密钥   : $PAIRING_FILE"
echo "   自启方式   : systemd (edgehop-client.service，开机自启)"
echo ""
echo " 接下来："
echo "   1) 确认 Mac 端 EdgeHop 已启动并已授权辅助功能"
echo "   2) 服务状态:  systemctl status edgehop-client"
echo "   3) 查看日志:  journalctl -u edgehop-client -f"
echo "   （若目标是登录界面也可控制，需 Omarchy 根分区非 LUKS 加密，"
echo "     或已配置 keyfile 自动解锁，见 docs/known-issues.md Issue #2）"
echo "=================================================="
