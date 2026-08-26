# agents

一句话：**会话总管**，负责创建、恢复、查找和关闭运行中的 Agent 会话。

```text
sessions + projects + presets + environment + llm + tools
                         │
                         ▼
                    agents
                         │
                         ├─ 管理 Conversation
                         └─ 把 Conversation 交给 Runner 执行
                                      │
                                      ▼
                                    loop
```

- 提供能力：`"agents"`
- 不亲自跑模型或工具；`loop` 是它登记进来的执行器。
- 先读：`plugin.go` → `service.go` → `conversation_manager.go`。
