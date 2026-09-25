# cmd

可执行程序入口：[`harness/`](harness/README.md) 启动 Web，`harness-desktop/` 启动 Wails 窗口。两者共用 `internal/backend` 的装配与关闭。

```text
cmd/harness / cmd/harness-desktop
  -> backend.Open -> appserver -> WebSocket / Wails Stream
  -> 退出时 backend.Close 逆序关闭
```

业务流程与具体实现在 `internal` 各领域；入口只接线。
