# loops

登记可复用的执行程序；Agent 的 Kind 决定选哪一个。

```text
入口 Register(react)
  -> Registry.Get(kind)
  -> Runner 调用 Loop.Run(Invocation)
```

`registry.go` 管登记与查询，`types.go` 定义 Loop、Invocation、事件和检查点。

Invocation 是 Runner 准备好的本轮输入。Loop 通过 Emit 交回输出，通过 Checkpoint 接收插话；活 Run、账本和取消收尾属于 Runner。
