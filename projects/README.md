# projects

一句话：**工作区管理处**，保存项目目录、最近模式/模型，并按项目归属会话。

```text
project-store + sessions
           │
           ▼
        projects
           │
           ▼
      agents / Web UI
```

- 依赖能力：`"project-store"`、`"sessions"`
- 提供能力：`"projects"`
- 项目只管“在哪里工作、记住什么选择”；会话细节仍在 `session`。
- 先读：`plugin.go` → `service.go` → `project.go`。
