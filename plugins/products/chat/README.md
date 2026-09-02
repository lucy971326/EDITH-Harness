# Chat 产品

【它是什么】

Chat Web 产品插件。提供项目 / 会话导航、真实消息区和 SSE 实时画面。

【提供能力】

当前不在 Host 上提供独立服务。

【使用能力】

Resolve `web`、`sessions`、`sessionSettings`、`llm`、`runner` 和 `events`。Chat 只发 Runner 命令、订阅 RunEvent；不保存 Run 或 FollowUp 的执行规则。

【填充插槽】

向 `web` 填入 Chat 产品和页面、History、SSE、发送、停止等 HTTP 路由。

## 实时对话

```text
GET History Snapshot JSON ─┐
                           ├→ apply() → paint() → 消息区
RunEvent → SSE JSON ───────┘

POST Run / Steer / FollowUp / Stop → Runner
```

一张会话页保持一条 SSE；服务端先订阅再确认连接，重连时 History Snapshot 同时带仍在运行的 Run，实时 Delta 才有稳定落点。慢客户端只会被断开，不能阻塞 Runner。

同一 Run 默认合为一张助手卡，按 `StepSeq / BlockSeq` 保留思考、正文与工具的顺序。只有 Runner 已把 Steer 用户消息落账时，才以该用户 Entry 为边界把同一 Run 切成前后两个片段；助手消息完成落账不会改变片段位置。模型和思考档位由 `llm.Client.Models()` 提供，首次发送前必须选好。
