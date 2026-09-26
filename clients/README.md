# clients

浏览器与 Desktop 共用一份 React 页面和 RPC Client；两种入口通过统一协议使用后台，只保存界面状态和服务端投影。

```text
web/（共用 React + RPC Client）
  ├─ 浏览器 WebSocket ───────────→ appserver
  └─ Wails Stream → desktop/ ───→ appserver
web/、test/（网络验收）→ contracts/（手写类型）
```

- [`contracts/`](contracts/README.md)：方法、参数、结果和事件的数据形状。
- [`web/`](web/README.md)：浏览器与 Desktop 共用的 React 页面、通信与投影。
- [`desktop/`](desktop/README.md)：Wails Stream 的 Go 传输适配。
- [`test/`](test/README.md)：无界面的真实网络验收 Client，不是正式 SDK。

业务事实归服务端；Client 不读取 Go 内部对象。
