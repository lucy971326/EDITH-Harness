# compact

把平台命令接到 Runner 的历史压缩能力。

```text
conversations -> commands.Call("compact") -> command.Run -> Runner.Compact
```

源码只有 `construct.go`：`New` 接收 Runner，入口把返回的命令登记进 commands。不另存状态，不在命令包装中实现摘要流程。
