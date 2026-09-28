# Desktop 传输

`wails.go` 把 Wails Stream 的一帧字节转换成一条完整 JSON-RPC 消息，限制为 16 MiB，写入停滞时关闭连接。业务方法与订阅由 `appserver.ServeStream` 处理。

```text
React RPC → Wails Stream → Stream → appserver.ServeStream
```

`platform.ts` 提供 Wails Stream 与系统浏览器能力，由 `clients/main.tsx` 注入共用 UI。

`notifications.go` 持有 Wails 原生通知服务、有界发送队列及后台只读事件订阅；前端只同步开关与权限状态。原生服务初始化失败时 Desktop 仍可启动。macOS 通知验收使用带 Bundle ID、本机临时签名的 `.build/EDITH.app`。

`lifecycle.go` 负责单窗口关窗、托盘与退出确认；共用 UI 只回报终端标签、草稿和保存状态，活 Run 由 Runner 只读查询。`window.go` 将普通窗口布局保存到 `~/.harness/desktop/window.json`，坏文件使用默认居中窗口。Windows 隐藏原生标题栏，由共用 UI 绘制窗口按钮；macOS 隐藏标题但保留原生交通灯。按钮经 `platform.ts` 调用 Wails 窗口 API，关闭仍触发 `lifecycle.go`，不进入 appserver 或领域服务。

窗口与进程启动见 [`cmd/harness-desktop`](../../cmd/harness-desktop/main.go)。

仓库根目录执行 `make desktop-run` 启动 Wails 开发模式：Vite 负责页面热更新，Wails 监视 Go 源码并重启桌面进程；`make desktop-build` 在 Windows 构建 `.build/EDITH.exe`，在 macOS 生成 `.build/EDITH.app`。Linux Desktop 不再支持，Linux Web 保留。产品版本只在 [`build/version.txt`](../../build/version.txt) 修改；Desktop PNG、ICO、ICNS 由现有 [`edith-icon.svg`](../public/edith-icon.svg) 生成，分别供运行时、Windows 和 macOS 使用。构建文件见 [`build/README.md`](../../build/README.md)。

```text
Makefile               开发命令入口
  └─ wails3 dev/build
      ├─ Taskfile.yml  Desktop 构建与运行
      └─ build/config.yml  dev 监听与启动顺序
clients/vite.config.ts  React 开发服务器
.build/                二进制输出
```

`make desktop-package` 在构建后生成 Windows NSIS 安装程序或 macOS DMG，工具与安装规则见 [`build/README.md`](../../build/README.md)。安装脚本归构建层，不调用 appserver；Windows 安装器通过 Wails 单实例 mutex 阻止运行时升级或卸载，因此修改 Desktop UniqueID 或升级 Wails 的单实例实现时，须同步核对 `build/windows/installer.nsi`。
