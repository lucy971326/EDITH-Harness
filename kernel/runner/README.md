# runner

【它是什么】一轮对话的运行外壳插件。

【提供能力】注册整份服务 `runner`，提供 `Run`、`Steer`、`Stop`。

【使用能力】Resolve `sessions`、`sessionSettings`、`agents`、`loops`、`events`。

【填充插槽】不填。

## 代码主干

```text
Run(sessionID, 用户消息)
  → 读取 SessionSettings
  → agents.Prepare
  → 用户消息先落账
  → 按 Kind 调 Loop.Run
  → 完整消息先落账，再发 events
  → Checkpoint 取 Steer
```

同一本 Session 同时只能有一个 Run；最终 Checkpoint 会原子关闭 Steer 入口。

Runner 是唯一写账者；Loop 不碰 Session，UI 不听账本。
