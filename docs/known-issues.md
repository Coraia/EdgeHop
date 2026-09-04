# 已知问题登记（Known Issues）

> 本文件登记已确认存在、暂未解决或等待业界方案的问题。每个条目含状态、
> 根因、已尝试方案与处置规则。状态：`open`（待处理）/ `resolved`（已解决）/
> `closed`（关闭，不做处理）。

---

## Issue #1：远程模式下三指上滑触发 Mac 的 Mission Control

- **状态**：`open`
- **登记日期**：2026-09-03
- **优先级**：中（体验问题，不影响核心键鼠/剪贴板功能）

### 现象

鼠标/控制权在 Omarchy（远程模式）时，在 Mac 触控板上做三指上滑，
Mac 端仍会弹出 Mission Control / App Exposé，即本地系统响应了本应只
作用于对端的手势。

### 根因（已确认）

三指上滑→Mission Control 是 **macOS 系统级原生手势**，由 WindowServer 在
独立事件路径上直接识别执行，**不产生 CGEvent 事件流**。因此：

- CGEventTap（无论 HID 还是 session 级别）**无法拦截/消费**该手势；
- 同类 KVM/远程工具同样受限：微软远程桌面官方确认无法在远程会话激活时
  屏蔽四指滑动手势；开源工具 Look Don't Touch 亦注明 Mission Control /
  热角走独立事件路径，CGEventTap 无法完全消费。

### 已尝试方案

1. **session-level CGEventTap 监听 NSEventTypeSwipe (type 31)**（参照
   synergy-gesture-bridge）：远程模式下消费 swipe 事件。结论：对普通
   swipe（三指左右切 Space 等）可能有效，但对绑定系统手势的三指上滑
   **无效**（系统根本不发出该事件）。
2. 仅消费鼠标移动/按键（现有 HID tap）：不影响系统手势路径。

### 潜在解法（未实施，待评估）

- **改系统手势绑定**：把 Mission Control 的触控板手势从三指改为四指或
  触发角/快捷键后，三指上滑从"系统手势"释放为普通 swipe 事件，即可被
  我们的 tap 拦截，并可进一步转发到 Omarchy（映射为 Hyprland workspace
  切换等）。缺点：需用户改系统设置，且四指仍会触发。
- **root 级 IOHIDManager 拦截原始触摸事件**：可彻底阻止系统手势，但需
  root helper、复杂度高，且会吞掉本地触摸输入，暂不采用。
- **检测到 Mission Control 激活后自动关闭**：可兜底，但会闪一下，体验一般。

### 处置规则

- 保持 `open`，等待业界出现应用层可拦截 macOS 系统手势的方案。
- **评估截止**：2026-12-02（登记后 90 天）。
- 若截止时仍无业界可行方案，关闭本 issue（`closed`），不做进一步处理。
- 若期间出现可行方案（如新的公开 API / 工具），重新评估并实施。

---

## Issue #2：Omarchy 开机 LUKS 解密界面无法用 Mac 键鼠（无法输密码）

- **状态**：`open`
- **登记日期**：2026-09-04
- **优先级**：高（每次开机/重启都必须物理键盘输解密密码，违背"全 Mac 键鼠控制"诉求）

### 现象

Omarchy 开机/重启后，屏幕停在 LUKS 解密密码界面（Plymouth 密码框，
用户最初误认为是系统登录界面）。此时 Mac 端把鼠标移到边缘无法切换过去，
Mac 键鼠无法在该界面输入密码；只能临时找物理键盘输入。登录进系统后
（网络就绪）KVM 全功能恢复正常。

### 根因（已确认）

- Omarchy 根分区为 **LUKS 全盘加密**（`/dev/nvme1n1p2` → `crypto_LUKS` →
  btrfs `/`），由内核 cmdline `cryptdevice=PARTUUID=...:root` + mkinitcpio
  **`encrypt` hook** + Plymouth 处理，开机 initramfs 阶段提示输入解密密码。
- 该界面发生在**根分区解锁之前**，uc-client 二进制位于加密根盘内，此阶段
  **不可执行**，Mac 端 server 因 `clientConnected=false` 也拒绝边缘切换。
  → **此阶段 Mac 键鼠在架构上无法控制，非自启动/网络配置问题。**
- 关联事实：开机 initramfs 阶段网络同样未就绪（NetworkManager 在根解锁后
  的 userspace 才启动），因此"网络是输密码后才好的"；systemd-analyze 显示
  该阶段耗时约 11 分钟（等密码输入，非故障）。
- 无 TPM2 芯片（`/sys/class/tpm` 不存在），TPM 自动解锁不可行。

### 已尝试方案 / 已验证事实

1. **systemd 服务自启（uc-client.service，multi-user.target）**：已部署，
   登录后/网络就绪后正常，但**无法覆盖 initramfs 阶段**（根未解锁二进制不可用）。
2. SDDM greeter 为 Wayland（`/etc/sddm.conf.d/10-wayland.conf`），libinput
   动态识别 uinput 设备 → **一旦越过 LUKS，SDDM 阶段注入大概率可用**（未最终验证）。
3. `hooks` 为 `encrypt`（非 `sd-encrypt`），keyfile 需走内核 cmdline `cryptkey=`
   或改用 `sd-encrypt`+crypttab；`/boot` 为独立 vfat ESP（2G，未加密），可放 keyfile。

### 潜在解法（未实施，待用户确认）

- **LUKS keyfile 自动解锁（推荐）**：生成随机 keyfile 放 `/boot`，
  `cryptsetup luksAddKey` 新增密钥槽 + 内核 cmdline 加 `cryptkey=/boot/<key>`
  → 开机免解密密码，直达 SDDM 登录界面，随后 uc-client 早期运行、Mac 键鼠可输密码。
  保留全盘加密；原密码槽保留，可逆。
- **移除 LUKS 加密**：彻底去掉开机密码，但需解密迁移、安全降级，风险高。
- **接受现状**：每次开机物理键盘输解密密码。

### 处置规则

- 保持 `open`，待用户在"LUKS keyfile 自动解锁"与"接受现状"之间决策后更新。
- 若实施 keyfile 方案：验证通过后标记 `resolved`；若验证失败且无法修复，
  回退并重新评估。
