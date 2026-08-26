# toolplugins/files

一句话：**写文件工具**，让模型在当前会话的工作目录中创建或修改文件。

```text
tools Registry
     ▲ Register(write_file)
     │
files Plugin
     │ 执行时领取
     ▼
会话作用域的 files
```

- 依赖能力：`"tools"`，执行时依赖会话作用域里的 `"files"`。
- 填充方式：登记 `write_file` 工具，不提供全局服务。
- 它不自己决定何时调用；模型通过 `loop` 的工具流程调用它。
- 先读：`plugin.go`。
