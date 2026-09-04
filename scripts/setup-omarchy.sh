#!/usr/bin/env bash
# 在 Omarchy（Arch + Hyprland / Wayland）上一键安装并自启 uc-client。
#
# 用法：
#   ./setup-omarchy.sh -s 192.168.1.10              # 自动匹配架构的客户端二进制（需在脚本同目录）
#   ./setup-omarchy.sh -s 192.168.1.10 -b ./uc-client-linux-amd64
#
# 做什么：
#   1) pacman 安装依赖   wl-clipboard hyprland-utils
#   2) 配置 /dev/uinput 访问（input 组 + udev 规则）
#   3) 安装 uc-client 到 /usr/local/bin
#   4) 写 systemd 服务（开机自启，覆盖登录界面）+ Hyprland 虚拟指针去加速
#
# 说明：uc-client 已支持在缺失会话环境变量时自动探测 Hyprland/Wayland
# socket，因此用 systemd 服务（multi-user.target）从开机一路管到登录会话，
# 登录界面（SDDM greeter）阶段即可被 Mac 键鼠控制，无需登录后再拉起。
set -euo pipefail

SERVER=""
BIN=""
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)   BIN_DEFAULT="./uc-client-linux-amd64" ;;
  aarch64|arm64) BIN_DEFAULT="./uc-client-linux-arm64" ;;
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

if ! command -v pacman >/dev/null 2>&1; then
  echo "未找到 pacman，本脚本仅支持 Arch 系（Omarchy）。" >&2
  exit 1
fi

echo "==> 1/4 安装依赖 (wl-clipboard)"
sudo pacman -S --needed --noconfirm wl-clipboard || { echo "安装 wl-clipboard 失败，请检查网络/源。" >&2; exit 1; }
# hyprctl 由 hyprland 包提供；hyprland-utils 仅在部分发行版存在，缺失不影响功能
sudo pacman -S --needed --noconfirm hyprland-utils 2>/dev/null \
  || echo "    (hyprland-utils 不存在，hyprctl 已随 hyprland 提供，跳过)"
command -v hyprctl >/dev/null 2>&1 || echo "    (警告: 未找到 hyprctl，边缘切回检测不可用)"

echo "==> 2/4 配置 /dev/uinput 访问权限"
sudo usermod -aG input "$USER"
RULES="/etc/udev/rules.d/99-universal-control.rules"
if [ ! -f "$RULES" ]; then
  echo 'KERNEL=="uinput", MODE="0660", GROUP="input", OPTIONS+="static_node=uinput"' | sudo tee "$RULES" >/dev/null
fi
sudo udevadm control --reload-rules
sudo udevadm trigger

echo "==> 3/4 安装 uc-client 到 /usr/local/bin"
sudo install -m 755 "$BIN" /usr/local/bin/uc-client

echo "==> 4/4 配置 systemd 开机自启 + Hyprland 虚拟指针去加速"
# --- systemd 服务：开机即启动（multi-user.target），覆盖登录界面阶段 ---
SERVICE="/etc/systemd/system/uc-client.service"
if [ -f "$SERVICE" ]; then
  echo "    $SERVICE 已存在，跳过创建（如需改参数请手动编辑）。"
else
  sudo tee "$SERVICE" >/dev/null <<EOF
[Unit]
Description=Universal Control client (boot + login keyboard/mouse)
After=network.target

[Service]
Type=simple
User=$USER
ExecStart=/usr/local/bin/uc-client -server $SERVER:24800 -edge right
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  echo "    已创建 $SERVICE。"
fi
sudo systemctl daemon-reload
sudo systemctl enable uc-client.service

# --- Hyprland 虚拟指针去加速（无登录自启；启动由 systemd 接管） ---
HYPR_DIR="$HOME/.config/hypr"
mkdir -p "$HYPR_DIR"

if [ -f "$HYPR_DIR/hyprland.lua" ]; then
  # --- Omarchy 风格 Lua 配置 ---
  INPUT="$HYPR_DIR/input.lua"
  if grep -q "universal-control" "$INPUT" 2>/dev/null; then
    echo "    input.lua 已包含 universal-control，跳过。"
  else
    cat >> "$INPUT" <<EOF

-- universal-control: 虚拟指针去加速（配合 hyprctl eval 可即时生效）
hl.device({
  name = "universal-control",
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
  if grep -q "universal-control" "$HYPR"; then
    echo "    hyprland.conf 已包含 universal-control，跳过。"
  else
    cat >> "$HYPR" <<EOF

# --- universal-control: 虚拟指针去加速 ---
input-device {
    name = universal-control
    accel_profile = flat
    sensitivity = 0
}
EOF
    echo "    已写入 input-device 去加速。"
  fi
fi

echo ""
echo "=================================================="
echo " 完成！已安装并配置 uc-client"
echo "   服务器地址 : $SERVER:24800"
echo "   客户端路径 : /usr/local/bin/uc-client"
echo "   自启方式   : systemd (uc-client.service，开机自启)"
echo ""
echo " 接下来："
echo "   1) 确认 Mac 端菜单栏应用/uc-server 已启动并已授权辅助功能"
echo "   2) 立即启动:  sudo systemctl start uc-client"
echo "   3) 查看日志:  journalctl -u uc-client -f"
echo "   （若目标是登录界面也可控制，需 Omarchy 根分区非 LUKS 加密，"
echo "     或已配置 keyfile 自动解锁，见 docs/known-issues.md Issue #2）"
echo "=================================================="
