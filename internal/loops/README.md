# loops

定义 Runner 与执行循环之间的输入、事件和检查点契约。

```text
backend 构造 react → Runner → Loop.Run(Invocation)
```

生产固定使用 [ReAct](react/README.md)，没有执行类型登记处。此包不反向依赖实现；最小 Run 契约允许测试注入可控执行逻辑。

`types.go` 定义 Invocation、事件和检查点。Loop 通过 Emit 交回输出，通过 Checkpoint 接收插话；活 Run、账本和取消收尾属于 Runner。
