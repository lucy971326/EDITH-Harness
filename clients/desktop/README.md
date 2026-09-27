# Desktop 传输

`wails.go` 把 Wails Stream 的一帧字节转换成一条完整 JSON-RPC 消息，限制为 16 MiB，写入停滞时关闭连接。业务方法与订阅由 `appserver.ServeStream` 处理。

```text
React RPC → Wails Stream → Stream → appserver.ServeStream
```

`platform.ts` 提供 Wails Stream 与系统浏览器能力，由 `clients/main.tsx` 注入共用 UI。

`lifecycle.go` 负责单窗口关窗、托盘与退出确认；共用 UI 只回报终端标签、草稿和保存状态，活 Run 由 Runner 只读查询。`window.go` 将普通窗口布局保存到 `~/.harness/desktop/window.json`，坏文件使用默认居中窗口。窗口行为不进入 appserver 或领域服务。

窗口与进程启动见 [`cmd/harness-desktop`](../../cmd/harness-desktop/main.go)。

仓库根目录执行 `make desktop-run` 启动 Wails 开发模式：Vite 负责页面热更新，Wails 监视 Go 源码并重启桌面进程；`make desktop-build` 构建正式二进制。

```text
Makefile               开发命令入口
  └─ wails3 dev/build
      ├─ Taskfile.yml  Desktop 构建与运行
      └─ build/config.yml  dev 监听与启动顺序
clients/vite.config.ts  React 开发服务器
.build/                二进制输出
```
