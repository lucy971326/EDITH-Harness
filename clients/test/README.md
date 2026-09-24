# test client

无界面的真实网络验收，用来检查协议整条链路。

```text
smoke.ts -> client.ts -> WebSocket -> appserver -> 后台服务
```

`client.ts` 处理初始化和类型化调用，`smoke.ts` 按场景执行验收。Go 网络测试负责启动隔离后台；它不是正式 SDK，也不保存业务状态。
