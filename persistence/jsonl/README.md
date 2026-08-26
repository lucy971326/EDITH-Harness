# persistence/jsonl

一句话：**会话账本的 JSONL 存储实现**。

```text
JSONL 文件
    ▲
    │ journal
    │
session Plugin
```

- 提供能力：`"journal"`
- `session` 只认识 `Journal` 接口，不认识 JSONL。
- 换存储方式时，替换这里，不用改 `session`。
- 先读：`plugin.go` → `journal.go`。
