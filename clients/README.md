# clients

> 用户实际接触 Harness 的地方。

```text
contracts/  前后端共同遵守的 TypeScript 数据形状
test/       没有界面的网络验收 Client
web/        正式 React 页面
```

Client 通过 WebSocket + JSON-RPC 调用后台，只保存界面状态和服务端投影，不拥有业务事实。

继续阅读：正式页面看 [`web/README.md`](web/README.md)，通信类型看 [`contracts/README.md`](contracts/README.md)。
