# tools

一句话：**工具登记处与执行流水线**，让模型安全地发现和调用工具。

```text
文件 / 其他工具插件
        │ Register(Tool)
        ▼
      tools Registry
        │ ExecuteCall
        ▼
       loop
```

- 提供能力：`"tools"`
- 本身没有具体工具；`toolplugins/files` 是一个填充物。
- 工具按“全局 + 会话遮蔽”登记，执行结果由 `loop` 记进账本。
- 先读：`plugin.go` → `registry.go` → `pipeline.go` → `types.go`。
