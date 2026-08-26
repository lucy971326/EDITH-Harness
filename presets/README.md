# presets

一句话：**Agent 模式管理处**，把系统提示、允许工具等配置保存为不可变版本。

```text
preset-store
     │
     ▼
  presets
     │ 锁定某个版本
     ▼
  agents → Conversation
```

- 依赖能力：`"preset-store"`
- 提供能力：`"presets"`
- 会话启动时锁定一个版本，之后编辑模式不会改写旧会话。
- 先读：`store.go` → `preset.go`。
