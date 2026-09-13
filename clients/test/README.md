# test client

> 无界面的真实 JSON-RPC 客户端，用来检查“网络整条链能不能走通”。

```text
client.ts  建立连接、初始化并调用类型化方法
smoke.ts   按真实顺序执行最小验收
```

它不是正式 SDK，也不保存业务状态。正式用户界面在 `clients/web`。
