# persistence/presetjson

一句话：**Agent 模式版本的 JSON 存储实现**。

```text
JSON 文件
    ▲
    │ preset-store
    │
presets Plugin
```

- 提供能力：`"preset-store"`
- `presets` 只认识 `Store` 接口，不认识 JSON 文件。
- 先读：`plugin.go` → `store.go`。
