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
#   4) 写 Hyprland 配置（虚拟指针去加速 + 登录后自启）
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

echo "==> 1/4 安装依赖 (wl-clipboard, hyprland-utils)"
sudo pacman -S --needed --noconfirm wl-clipboard hyprland-utils

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

echo "==> 4/4 配置 Hyprland（虚拟指针去加速 + 登录自启）"
HYPR="$HOME/.config/hypr/hyprland.conf"
mkdir -p "$(dirname "$HYPR")"
touch "$HYPR"
if grep -q "universal-control" "$HYPR"; then
  echo "    Hyprland 配置已包含 universal-control，跳过写入。"
  echo "    如需修改服务器地址，请手动编辑 $HYPR 中的 exec-once 行。"
else
  cat >> "$HYPR" <<EOF

# --- universal-control: 虚拟指针去加速 ---
input-device {
    name = universal-control
    accel_profile = flat
    sensitivity = 0
}

# --- universal-control: 登录后自启 uc-client ---
exec-once = /usr/local/bin/uc-client -server $SERVER:24800
EOF
  echo "    已写入 input-device 去加速 + exec-once 自启。"
fi

echo ""
echo "=================================================="
echo " 完成！已安装并配置 uc-client"
echo "   服务器地址 : $SERVER:24800"
echo "   客户端路径 : /usr/local/bin/uc-client"
echo ""
echo " 接下来："
echo "   1) 重新登录 Hyprland（让 input 组生效），或临时执行: newgrp input"
echo "   2) 先手动测试:  /usr/local/bin/uc-client -server $SERVER:24800"
echo "   3) 确认 Mac 端菜单栏应用/uc-server 已启动并已授权辅助功能"
echo "=================================================="
