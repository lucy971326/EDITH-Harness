# appserver

> Harness 的网络门卫：接请求、做校验、转交业务、发回结果。

```text
Browser / Client
       ↓ WebSocket + JSON-RPC
   appserver
       ├─ Harness 请求 → products/harness
       └─ 公共请求    → Agent / Model / Skill / Command
```

## 从哪里读

```text
server.go         Server 依赖、方法表与调用分发
listener.go       HTTP 监听的启动和关闭
lifecycle.go      工作准入、关闭广播与收尾等待
websocket.go      页面/RPC 分流、WebSocket 握手与消息流适配
connection.go     单个 Client 的协议状态、通知与订阅
harness.go        Harness 方法登记与处理
harness_run.go    Run 的订阅和事件转发
harness_types.go  Harness 网络参数与结果
agents.go 等      公共服务接口
```

```text
listener → websocket → connection → Server.Call → 具体处理方法
              │
              └─ lifecycle 统一保证关闭安全
```

## 边界

- 做：协议、Schema 校验、Origin 安全、连接和订阅清理。
- 不做：聊天业务、账本存储、运行决策。
- `websocket.go` 不直接操作锁；并发规则集中在 `lifecycle.go`。
- 断开连接不会停止已接受的 Run。
