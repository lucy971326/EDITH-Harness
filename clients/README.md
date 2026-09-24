# clients

Client 通过统一协议使用后台，只保存界面状态和服务端投影。

```text
web（正式界面） / test（网络验收）
  -> contracts（手写类型）-> WebSocket JSON-RPC -> appserver
```

- [`contracts/`](contracts/README.md)：方法、参数、结果和事件的数据形状。
- [`web/`](web/README.md)：React 页面、通信与投影。
- [`test/`](test/README.md)：无界面的真实网络验收 Client，不是正式 SDK。

业务事实归服务端；Client 不读取 Go 内部对象。
