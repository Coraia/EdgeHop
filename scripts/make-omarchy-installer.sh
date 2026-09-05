#!/usr/bin/env bash
# 打包 Omarchy 端安装器：setup-omarchy.sh + 两个架构的客户端二进制
# 产出: dist/edgehop-omarchy-install.tar.gz
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> building client binaries"
make build-client >/dev/null

echo "==> assembling tarball"
mkdir -p dist/edgehop-omarchy
cp scripts/setup-omarchy.sh dist/edgehop-omarchy/
cp bin/edgehop-client-linux-amd64 bin/edgehop-client-linux-arm64 dist/edgehop-omarchy/
chmod +x dist/edgehop-omarchy/setup-omarchy.sh

tar -C dist -czf dist/edgehop-omarchy-install.tar.gz edgehop-omarchy
rm -rf dist/edgehop-omarchy
shasum -a 256 dist/edgehop-omarchy-install.tar.gz > dist/edgehop-omarchy-install.tar.gz.sha256

echo "==> done: dist/edgehop-omarchy-install.tar.gz"
echo "    拷到 Omarchy 后解压并执行:"
echo "    tar xzf edgehop-omarchy-install.tar.gz && cd edgehop-omarchy && ./setup-omarchy.sh -s <Mac IP>"
echo "    按提示粘贴 Mac 菜单中复制的配对码。"
