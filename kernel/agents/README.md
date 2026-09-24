# agents

管理 Agent 设置，准备本轮的执行类型、工具与系统提示词。

```text
Agent 设置 + 当前工具 / Skills
  -> Service.Prepare -> PreparedAgent -> Runner

Service -> fileStore -> persist -> agents/<id>.json
```

阅读顺序：`types.go` 看数据与存储契约，`service.go` 看设置校验和 Prepare，`store.go` 看 JSON 格式。

`references` 锁协调 Agent 删除与会话引用写入、目录读取。普通工具来自 Agent 选择；MCP、Skills 按作用域发现。不执行 Loop，不写对话账本，不迁移旧工具名。
