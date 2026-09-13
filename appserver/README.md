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
server.go         方法表与 Server 生命周期
websocket.go      HTTP、静态页面与 WebSocket 入口
connection.go     单个 Client 的连接、通知与订阅
harness.go        Harness 方法登记与处理
harness_run.go    Run 的订阅和事件转发
harness_types.go  Harness 网络参数与结果
agents.go 等      公共服务接口
```

## 边界

- 做：协议、Schema 校验、Origin 安全、连接和订阅清理。
- 不做：聊天业务、账本存储、运行决策。
- 断开连接不会停止已接受的 Run。
