# react

默认 ReAct 执行循环，源码集中在 `react.go`。

```text
Runner -> Run -> 模型流 -> 工具调用 -> 模型流 ... -> 最终回答
             +-> Emit 输出
             +-> Checkpoint 接收外部输入
```

`New` 接收 LLM 与工具登记处；入口登记 `react` Kind。一次模型输出及全部工具结果之间不插入外部消息，在步骤后的检查点消费输入。

取消时不再执行剩余工具，但为已发出的调用补齐结果。这里不读取 SessionSettings、不直接写账本、不向浏览器发通知；这些由 Runner 处理。
