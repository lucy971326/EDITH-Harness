# commands

一句话：**人类命令登记处**，让人用 `/` 直接调用插件，不经过模型和工具流水线。

```text
能力插件
    │ Register(Command)
    ▼
 commands Registry
    │ Execute
    ▼
   Web / 其他界面
```

- 提供能力：`"commands"`
- 本身没有具体命令；谁拥有能力，谁登记自己的命令。
- 以 `/` 开头的提交行只走这里：不认识就报错，绝不变成聊天。
- 先读：`plugin.go` → `service.go` → `registry.go` → `execute.go`。
