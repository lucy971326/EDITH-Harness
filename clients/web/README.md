# Web 接入

浏览器平台能力与 Go 网络传输放在本目录，共用页面见 [UI](../ui/README.md)。

```text
platform.ts -> WebSocket /rpc -> server.go + websocket.go -> appserver.ServeStream
```

- `platform.ts`：计算同源 RPC 地址；打开外部网址到新标签。
- `server.go`：回环 HTTP 监听、静态页面分发、握手准入与传输收尾。
- `websocket.go`：Host / Origin 校验、16 MiB 消息限制、JSON 帧收发与写入超时。
- `rpc-proxy.ts`：Vite 开发代理的 Origin 校验；只允许同源页面升级。
- `test/network.ts`：使用共用 UI Client 的端到端网络验收；Go 协议与关闭测试见 `websocket_test.go`。

启动与关闭由 [cmd/harness](../../cmd/harness/README.md) 接线。关闭先停止准入并取消 HTTP／WebSocket，再等待已接入的 RPC 清理；业务状态和已接受的 Run 归后台。
