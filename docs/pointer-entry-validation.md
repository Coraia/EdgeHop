# Omarchy 入屏指针抖动：失败路径与验收

## 修改前的失败路径

- 入屏定位在 goroutine 中执行，相对移动反馈校正与真实触控板输入同时写入 uinput，把指针不断拉回共享边缘。
- 相对定位受到 Hyprland 加速、IPC 延迟影响，40 次校正可能反复超调。
- 快速返回/再次进入时，旧定位任务或旧边缘观察任务继续影响新会话。
- Hyprland 0.55+ Lua dispatcher 与旧 movecursor 语法不同；命令退出成功也可能返回错误文本。
- 定位失败、IPC 卡住或无会话时，不能退回持续相对校正，也不能无限阻塞输入。
- 入屏位置应保持左右布局和屏幕高度比例；入屏后真实相对移动必须保留。
- 自动化轨迹通过不能证明真实触控板手感、Mac HID 捕获或屏幕像素已人工验收。

## 实机 E2E

`tools/pointer-entry-e2e` 运行临时 TLS 服务端与实际 Linux 客户端，经真实
uinput → Hyprland 注入确定性移动，并采集 cursorpos。使用随机临时配对密钥、
独立虚拟设备，不读取实际配对密钥或保存剪贴板内容。

保存旧版失败、候选版 JSON 轨迹、客户端日志、命令退出码及二进制 SHA256。
开发时只运行旧版的单个复现；修改完成后运行候选完整矩阵一次。

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/pointer-entry-e2e ./tools/pointer-entry-e2e
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/edgehop-client ./cmd/edgehop-client
# 拷贝到 Omarchy 后，在登录的 Hyprland 会话中运行：
./pointer-entry-e2e -client ./edgehop-client -output ./pointer-evidence
# -repro-only 仅复现右侧入屏后的快速移动
```

完整矩阵检查左右入屏静止定位、高度比例、立即快速移入后单向轨迹、停止后
无持续回拉、快速返回后再次进入。结果保留实际坐标及断言，不把自动化输入
写成物理触控板操作验收。

## 2026-10-05 验收结果

- 旧实现的单次快速入屏复现失败，记录 38 条轨迹断言失败。
- 候选客户端在 Hyprland 0.56.2 上完成全部 5 项 E2E：左右快速入屏、
  左右静止定位与高度比例、快速返回后重入，全部通过。
- Linux amd64、arm64 构建与客户端/E2E 工具的 `go vet` 通过。
- Omarchy 客户端已更新并重新连接 Mac；旧客户端已备份。
- 用户实际操作触控板后确认：“现在不再跳了”，并授权 commit/push。
- 轨迹、两版测试二进制、源码差异、部署记录和 SHA256 清单保存在本地
  `dist/pointer-validation-20261005/`（忽略生成产物，不提交设备日志）。

自动化验证使用确定性网络输入；物理触控板结论来自用户反馈。
旧 Hyprland dispatcher 回退与 IPC 超时路径未在本次实机 E2E 中覆盖。
