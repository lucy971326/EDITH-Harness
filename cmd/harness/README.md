# harness 入口

从 `main.go` 的 `run()` 开始读：识别文件助手模式，打开共享后台，再启动 Web 监听。

```text
backend.Open → 锁定 ~/.harness → 构造服务 → 绑定 appserver
cmd/harness   → 监听 127.0.0.1:8888 → 默认打开浏览器
```

- `RunFileWorker`：最先识别内部文件助手模式，不启动完整服务。
- `run`：打开后台；所有登记成功后才监听。
- `openBrowser`：默认打开页面；`make run` 使用 `--no-browser`，由 Vite 打开支持热更新的开发页。共享装配见 [`backend`](../../internal/backend/open.go)。

退出时 appserver 最先关闭，随后逆序释放服务；关闭错误汇总返回。这里没有 Host 服务表或 Product 层。
