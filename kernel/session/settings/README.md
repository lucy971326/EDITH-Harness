# session/settings

> 一场会话“下一轮如何运行”的设置契约。

```text
SessionSettings
├─ AgentID
├─ Model
├─ ReasoningEffort
└─ Workspace
```

它与消息账本分开保存。这里定义数据和 Store 接口；实际文件读写由 `kernel/persist` 提供。
