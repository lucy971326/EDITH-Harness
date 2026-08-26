# session

一句话：**会话账本管家**，创建、读取和追加每段会话的事实记录。

```text
JSONL Journal
     │
     ▼
  session Store
     │  EventAppended
     ▼
agents / loop / Web UI
```

- 依赖能力：`"journal"`
- 提供能力：`"sessions"`
- 账本不懂模型和界面；它只保证事件可追溯，并在每次记账后广播。
- 先读：`plugin.go` → `store.go` → `session.go` → `types.go`。
