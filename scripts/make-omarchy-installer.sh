#!/usr/bin/env bash
# 打包 Omarchy 端安装器：setup-omarchy.sh + 两个架构的客户端二进制
# 产出: dist/omarchy-install.tar.gz
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> building client binaries"
make build-client >/dev/null

echo "==> assembling tarball"
mkdir -p dist/omarchy-install
cp scripts/setup-omarchy.sh dist/omarchy-install/
cp bin/uc-client-linux-amd64 bin/uc-client-linux-arm64 dist/omarchy-install/
chmod +x dist/omarchy-install/setup-omarchy.sh

tar -C dist -czf dist/omarchy-install.tar.gz omarchy-install
rm -rf dist/omarchy-install

echo "==> done: dist/omarchy-install.tar.gz"
echo "    拷到 Omarchy 后解压并执行:"
echo "    tar xzf omarchy-install.tar.gz && cd omarchy-install && ./setup-omarchy.sh -s <Mac IP>"
