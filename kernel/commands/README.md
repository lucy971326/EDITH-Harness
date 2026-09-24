# commands

按名称登记平台命令，向 Client 提供命令列表。

```text
入口 Register(compact)
  -> Registry
  <- conversations.CallCommand -> Call -> 命令.Run
```

`types.go` 定义命令契约，`registry.go` 负责登记、查询和分发。`/` 是 Client 的输入交互；压缩业务属于 Runner，不在登记处实现。
