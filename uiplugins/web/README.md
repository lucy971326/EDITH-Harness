# uiplugins/web

一句话：**本地网页工作台**，把已有能力显示出来并转发用户操作。

```text
projects + presets + sessions + agents + llm + tools + commands
                         │
                         ▼
                       Web UI
                         │
              HTTP / HTMX / SSE
                         ▼
                       浏览器
```

- 依赖上述领域能力，提供能力：`"ui"`。
- 不执行 Agent；它只开会话、发消息、转发斜杠命令、选择模型、展示账本和刷新界面。
- 以 `/` 开头的提交走 `commands.Execute`，绝不 `SubmitFollowup`。
- SSE 只提示刷新：聊天记录与输入区分开刷新，避免流式输出打断取消按钮。
- 先读：`plugin.go` → `server.go` → `routes.go` → `handlers_chat.go` → `page_data.go`。
