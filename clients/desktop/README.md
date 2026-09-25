# Desktop 传输

`stream.go` 把 Wails Stream 的一帧字节转换成一条完整 JSON-RPC 消息，限制为 16 MiB，写入停滞时关闭连接。业务方法与订阅由 `appserver.ServeStream` 处理。

```text
React RPC → Wails Stream → Stream → appserver.ServeStream
```

窗口与进程启动见 [`cmd/harness-desktop`](../../cmd/harness-desktop/main.go)。

仓库根目录执行 `make desktop-run` 启动 Wails 开发模式：Vite 负责页面热更新，Wails 监视 Go 源码并重启桌面进程；`make desktop-build` 构建正式二进制。

```text
Makefile               开发命令入口
  └─ wails3 dev/build
      ├─ Taskfile.yml  Desktop 构建与运行
      └─ build/config.yml  dev 监听与启动顺序
clients/web/vite.config.ts  React 开发服务器
.build/                二进制输出
```
