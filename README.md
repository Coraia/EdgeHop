# universal_control

让 **Mac mini 的触控板和键盘控制 Omarchy 台式机**（Linux / Arch + Hyprland / Wayland），并在两台电脑间**双向同步剪贴板**——一个软件级 KVM（Keyboard-Video-Mouse over LAN）。

配合两台机器共用的 PBP（画中画/双画面）显示器：**Omarchy 在左半屏、Mac mini 在右半屏**，鼠标移到两台屏幕之间的边缘即可"穿越"切换，就像在用一台电脑。

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

```
┌───────────────┬───────────────┐
│    Omarchy    │   Mac mini    │  ← 同一台显示器 PBP 分屏
│  (左半屏)      │  (右半屏)      │
│  Linux/Arch   │  物理键鼠在这边  │
└───────┬───────┴───────┬───────┘
        │  局域网 TCP    │
  uc-client          uc-server
  (/dev/uinput + wl-clipboard)   (CGEventTap)
```

## 功能特性

- **边缘穿越**：鼠标推到屏幕边缘，带"粘住 → 挣脱"的苹果风视觉反馈（全高 2px 亮线 + 柔和光晕，随贴近收窄），进入/退出时 Y 坐标按屏高比例平移，光标出现在对端对应位置而非固定点。
- **热键切换**：默认 **⌘⇧空格** 在任意一侧、任意位置手动切换控制权。
- **剪贴板同步**：纯文本双向复制粘贴（Mac `pbpaste`/`pbcopy` ↔ Linux `wl-copy`/`wl-paste`）。
- **登录界面可用**：Linux 端以 systemd 服务自启，SDDM greeter 阶段即可被 Mac 键鼠控制（受 LUKS 加密盘限制，见[已知问题](docs/known-issues.md)）。

## 工作原理

- **uc-server**（跑在 Mac mini）：通过 macOS `CGEventTap` 全局捕获触控板/键盘事件。
  - 本地模式：事件正常作用于 Mac；
  - 光标到达 Mac **左边缘**（Omarchy 在 Mac 左侧）或按下热键 → 进入远程模式，事件被抑制并转发给 Omarchy；
  - 剪贴板通过 `pbpaste`/`pbcopy` 轮询同步。
- **uc-client**（跑在 Omarchy）：纯 Go 直写 `/dev/uinput` 注入键鼠事件（免 cgo、免 ydotool 守护进程），用 `wl-copy`/`wl-paste` 同步剪贴板；通过 `hyprctl cursorpos` 检测光标到 Omarchy **右边缘**，请求切回 Mac。
  - **环境自动探测**：缺失 `HYPRLAND_INSTANCE_SIGNATURE` / `WAYLAND_DISPLAY` 时自动从 `/run/user/<uid>/hypr` 与 `wayland-*` socket 探测，因此可由 systemd 在登录前启动、登录后无缝接管，无需重启。
- 协议：自研长度前缀二进制帧（鼠标移动/点击/滚轮/键盘/剪贴板/切换），端口默认 `24800`。

## 构建

在 Mac mini 上（已装 Go 1.2x）：

```bash
make build          # 产出 bin/uc-server、bin/uc-client-linux-amd64、bin/uc-client-linux-arm64
make bundle         # 生成 macOS 菜单栏应用 dist/universal-control.app
make omarchy-installer  # 生成 Omarchy 一键安装包 dist/omarchy-install.tar.gz
```

## Mac mini 端部署（uc-server）

### 方式 A：菜单栏应用（推荐，免开终端）

```bash
make bundle
open dist/universal-control.app
```

- 运行后右上角菜单栏出现 Universal Control 图标，服务器同进程自动启动；
- **授予辅助功能（Accessibility）权限**：系统设置 → 隐私与安全性 → 辅助功能，勾选 `universal-control`。未授权时应用会静默等待，不再反复弹授权框；
- 菜单可查看服务器/客户端/模式状态、打开辅助功能设置、打开日志（`~/Library/Logs/universal-control.log`）、勾选"登录时自动启动"（写入 LaunchAgent）、退出；
- 应用为 `LSUIElement`，不占 Dock。
- **边缘方向配置**：写入 `~/Library/Application Support/universal-control/config.json`，默认布局（Omarchy 在左）为：

  ```json
  {"edge": "left"}
  ```

  命令行参数优先于配置文件。
- **默认切换热键 ⌘⇧空格**：即使边缘检测未触发也能切回；可在 config.json 用 `"switchKeys": [55,56,49]` 覆盖（macOS 键码）。

### 方式 B：命令行（开发/调试用）

```bash
./bin/uc-server -listen 0.0.0.0:24800 -edge left
```

同样需要给 `uc-server`（或终端 App）授予辅助功能权限。

### uc-server 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `-listen` | `0.0.0.0:24800` | 监听地址 |
| `-edge` | `right` | Omarchy 在 Mac 的哪一侧：`right`/`left` |
| `-edge-sensitivity` | `2.0` | 触发切换的边缘像素余量 |
| `-switch-keys` | 空 | 手动切换热键的 macOS 键码，逗号分隔，如 `55,56,49`（Cmd+Shift+空格） |
| `-clip-interval` | `500ms` | 剪贴板轮询间隔 |

## Omarchy 端部署（uc-client）

**一键安装**：在 Mac 上执行 `make omarchy-installer` 生成 `dist/omarchy-install.tar.gz`，拷到 Omarchy 后：

```bash
tar xzf omarchy-install.tar.gz && cd omarchy-install
./setup-omarchy.sh -s <Mac mini 的IP>        # 自动匹配架构；也可 -b 指定二进制
```

脚本会自动完成：
1. `pacman` 安装 `wl-clipboard`、`hyprland-utils`；
2. 配置 `/dev/uinput` 访问（加入 `input` 组 + udev 规则）；
3. 安装 `uc-client` 到 `/usr/local/bin`；
4. 创建 **systemd 服务 `uc-client.service`**（`multi-user.target` 开机自启，覆盖登录界面），并写入 Hyprland 虚拟指针去加速配置。

**手动安装**（等价步骤，供排查）：

1. **安装依赖**（Arch）：

   ```bash
   sudo pacman -S --needed wl-clipboard hyprland-utils
   ```

2. **放行 uinput**（一次配置）：

   ```bash
   sudo usermod -aG input $USER
   echo 'KERNEL=="uinput", MODE="0660", GROUP="input", OPTIONS+="static_node=uinput"' | sudo tee /etc/udev/rules.d/99-universal-control.rules
   sudo udevadm control --reload-rules && sudo udevadm trigger
   # 重新登录使 input 组生效
   ```

3. **systemd 服务**（开机自启，覆盖登录界面；登录后仍由它管理，无需会话内拉起）：

   ```ini
   # /etc/systemd/system/uc-client.service
   [Unit]
   Description=Universal Control client (boot + login keyboard/mouse)
   After=network.target

   [Service]
   Type=simple
   User=$USER
   ExecStart=/usr/local/bin/uc-client -server <Mac mini 的IP>:24800 -edge right
   Restart=always
   RestartSec=3

   [Install]
   WantedBy=multi-user.target
   ```

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now uc-client.service
   journalctl -u uc-client -f   # 查看日志
   ```

4. **让虚拟指针平滑**：在 `~/.config/hypr/hyprland.conf` 中加：

   ```ini
   input-device {
       name = universal-control
       accel_profile = flat
       sensitivity = 0
   }
   ```

5. 参数：

   | 参数 | 默认 | 说明 |
   |---|---|---|
   | `-server` | （必填） | Mac mini 地址，如 `192.168.1.10:24800` |
   | `-device` | `universal-control` | uinput 设备名（与 hyprland.conf 一致） |
   | `-edge` | `right` | Omarchy 的哪一侧对着 Mac：`right`（Omarchy 在左）、`left`（Omarchy 在右） |
   | `-edge-margin` | `8` | 边缘多少像素内触发切回 Mac（逻辑像素） |
   | `-clip-interval` | `500ms` | 剪贴板轮询间隔 |
   | `-edge-poll` | `40ms` | 光标位置轮询间隔 |

   注意：Omarchy 的 `hyprctl cursorpos` 返回**逻辑坐标**（受显示器 scale 影响，如 1.25x 时 1920 物理宽 → 逻辑宽 1536）。客户端已按逻辑尺寸计算边缘，无需手动换算。

## 使用

- **Mac → Omarchy**：把鼠标移到 Mac 屏幕**左边缘**（Omarchy 在 Mac 左侧时）；键鼠随即控制 Omarchy，Mac 光标自动隐藏。
- **Omarchy → Mac**：把鼠标移到 Omarchy 屏幕**右边缘**，控制权切回 Mac。
- **热键切换**：按 **⌘⇧空格** 可在任意一侧、任意位置手动切换（默认开启）。
- **剪贴板**：任一台上复制文本，另一台可直接粘贴（纯文本，双向）。

## 视觉与交互（边缘穿越）

参考苹果 Universal Control 的克制风格：光标贴近边缘时浮现**全高 2px 亮白线 + 柔和光晕**（随贴近收窄），随后需"粘住 → 挣脱"才进入对端，防止误触；进入/退出时 Y 坐标按屏高比例映射，光标出现在对端对应高度。全部参数可在 `internal/server/engine.go` / `config.go` 调整。

## 已知问题

见 [docs/known-issues.md](docs/known-issues.md)：

- **Issue #1**：远程模式下三指上滑触发 Mac 的 Mission Control（macOS 系统手势，CGEventTap 无法拦截，等待业界方案）。
- **Issue #2**：Omarchy 根分区为 LUKS 加密盘时，开机 LUKS 解密界面在 initramfs 阶段，uc-client 不可执行，Mac 键鼠无法在该阶段输密码（需物理键盘或配置 keyfile 自动解锁）。

## 目录结构

```
cmd/uc-server/          macOS 端入口（命令行）
cmd/uc-tray/            菜单栏应用入口（systray + 内嵌服务器）
cmd/uc-client/          Linux 端入口
internal/protocol/      网络协议（两端共用，含单测）
internal/keymap/        macOS 键码 → Linux evdev 键码（含单测）
internal/server/        macOS 实现（CGEventTap、剪贴板、切换引擎、状态回调、边缘特效 overlay）
internal/client/        Linux 实现（uinput 注入、wl-clipboard、边缘检测、环境自动探测）
tools/genicon/          菜单栏模板图标生成器
scripts/bundle-macos.sh .app 打包脚本（LSUIElement + ad-hoc 签名）
scripts/setup-omarchy.sh Omarchy 一键安装/自启脚本（systemd 服务）
scripts/make-omarchy-installer.sh Omarchy 安装器 tar 打包
docs/known-issues.md    已知问题登记
```

## 许可证

[MIT](LICENSE)
