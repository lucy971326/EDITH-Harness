# persistence/projectjson

一句话：**项目元数据的 JSON 存储实现**。

```text
JSON 文件
    ▲
    │ project-store
    │
projects Plugin
```

- 提供能力：`"project-store"`
- `projects` 只认识 `Store` 接口，不认识 JSON 文件。
- 先读：`plugin.go` → `store.go`。
