# session/settings

保存一场会话下一轮如何运行，与对话账本分开。

```text
AgentID / Model / ReasoningEffort / Workspace / PermissionMode
  -> Store.Put -> persist -> sessions/<id>/settings.json
  -> Store.For -> Runner 本轮快照
```

`types.go` 定义数据与存储契约，`store.go` 实现 JSON 读写和 Agent 引用查询。

新设置保存时补默认权限模式；读取缺字段或未知模式报错，不迁移旧数据。Agent 引用写入与删除由 agents.Service 协调；一次审批的额外权限不存这里。
