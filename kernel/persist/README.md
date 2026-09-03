# persist

【它是什么】内置的 JSONL 持久化插件。

【提供能力】同一份对象注册为 `sessionPersistence`、`sessionSettings`、`agentStore`。

【使用能力】无。

【填充插槽】不填。

## 代码主干

```text
Start(Dir)
  → 打开 JSONL 存储
  → 注册三把服务键

账本节点       → sessions/<session>/messages.jsonl
会话元数据     → sessions/<session>/meta.json
会话设置       → sessions/<session>/settings.json
自建 Agent     → <agent>.agent.json
```

它只负责磁盘数据，不认识 Runner 和对话流程。
