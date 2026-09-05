# EdgeHop — macOS 键鼠控制 Omarchy 桌面（软件 KVM）

.PHONY: all build build-server build-client bundle dmg omarchy-installer icon test clean

all: build

# macOS 端 + Linux 两端架构的客户端
build: build-server build-client

build-server:
	go build -trimpath -ldflags="-s -w" -o bin/edgehop-server ./cmd/edgehop-server

build-client:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/edgehop-client-linux-amd64 ./cmd/edgehop-client
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/edgehop-client-linux-arm64 ./cmd/edgehop-client

# macOS 菜单栏应用（.app bundle，免终端）
bundle:
	./scripts/bundle-macos.sh

dmg: bundle
	./scripts/make-macos-dmg.sh

# Omarchy 端一键安装器（脚本 + 双架构客户端）
omarchy-installer:
	./scripts/make-omarchy-installer.sh

icon:
	go run ./tools/genicon

test:
	go test ./...
	bash ./scripts/setup-omarchy-test.sh

clean:
	rm -rf bin dist
