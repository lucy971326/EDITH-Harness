# runner

【它是什么】一轮对话的运行外壳插件。

【提供能力】注册整份服务 `runner`，提供 `Start`、`Run`、`Steer`、`FollowUp`、`Stop`。

【使用能力】Resolve `sessions`、`sessionSettings`、`agents`、`loops`、`events`。

【填充插槽】不填。

## 代码主干

```text
Start(sessionID, 用户消息)
  → 同步占住这本 Session，后台执行 Run

Run(sessionID, 用户消息)
  → 读取 SessionSettings
  → agents.Prepare
  → 用户消息先落账
  → 按 Kind 调 Loop.Run
  → 完整消息先落账，再发布稳定 RunEvent
  → Checkpoint 取 Steer
  → 继续执行已排入的 FollowUp
```

同一本 Session 同时只能有一个活 Run；`FollowUp` 由 Runner 按顺序启动下一轮，`Stop` 只取消当前轮、不清空 FollowUp。最终 Checkpoint 会原子关闭 Steer 入口。

Runner 是唯一写账者；Loop 不碰 Session，UI 不听账本。对外 `RunEvent` 是稳定契约，不泄漏 Loop 内部事件；关闭 Runner 会取消并等待它管理的全部 Run。
