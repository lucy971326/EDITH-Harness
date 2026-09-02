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
GET History JSON ─┐
                  ├→ 一份 JS paint() → 消息区
RunEvent → SSE ───┘

POST Run / Steer / FollowUp / Stop → Runner
```

一张会话页保持一条 SSE；重连时重新同步 History。慢客户端只会被断开，不能阻塞 Runner。模型和思考档位由 `llm.Client.Models()` 提供，首次发送前必须选好。
