# harness 入口

从 `main.go` 的 `run()` 开始读：依赖按顺序构造，每取得长期资源立即安排 `defer` 收尾。

```text
文件 / 模型 / machine / 登记处
  -> Hook / 审批 / 工具 / Skills
  -> Agents -> Runner -> Subagents -> conversations
  -> appserver 绑定 -> 监听 127.0.0.1:8888
```

- `RunFileWorker`：最先识别内部文件助手模式，不启动完整服务。
- `run`：显式传入依赖；所有登记成功后才监听。
- `userDataDir / openBrowser`：定位数据目录、打开页面。

退出时 appserver 最先关闭，随后逆序释放服务；关闭错误汇总返回。这里没有 Host 服务表或 Product 层。
