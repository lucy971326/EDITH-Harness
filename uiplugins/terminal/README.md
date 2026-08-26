# uiplugins/terminal

一句话：**暂停中的 TUI 占位插件**。

```text
compose 选择 terminal
          │
          ▼
      ui-terminal
          │
          ▼
  "TUI 已暂停" 的提示
```

- 提供能力：`"ui"`
- 它现在不提供真正的终端界面，只是避免旧 TUI 依赖已移除的架构。
- 真正可用的界面是 `uiplugins/web`。
- 先读：`plugin.go`。
