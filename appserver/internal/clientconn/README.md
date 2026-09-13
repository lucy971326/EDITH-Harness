# clientconn

```text
一个 WebSocket Client
├─ 初始化
├─ 收请求 / 回响应
├─ 发通知
└─ 断线清理订阅
```

它只管一条连接。每个请求返回接收循环前，先让 `internal/rpc` 完成校验和必要的排队；业务执行仍然异步，因此 Stop 不会被阻塞。

断线会取消仍依赖连接的请求，但不会停止 Product 已接受的 Run。
