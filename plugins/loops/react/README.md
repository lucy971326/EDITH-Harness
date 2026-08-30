# react

【它是什么】默认的 ReAct 运行范式插件。

【提供能力】不注册整份服务。

【使用能力】Resolve `llm`、`tools`、`loops`。

【填充插槽】向 `loops` 登记 `react`。

## 代码主干

```text
Runner 传入 Invocation
  → 取得本轮 Tool Schema
  → llm.Stream
  → Emit 完整 assistant 消息
  → 顺序执行 Tool，并 Emit 结果
  → Checkpoint 接收 Steer
  → 无 Tool、无 Steer时结束
```

最终 Checkpoint 会关闭 Steer 入口，避免消息成功入队后丢失。

它不读取 SessionSettings、不写账本；落账由 Runner 负责。
