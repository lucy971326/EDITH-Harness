# subagent tools

> 把 `kernel/subagents` 的能力包装成模型可调用的工具。

```text
subagent_options  查看可用 Agent / 模型
subagent_spawn    派出任务
subagent_send     追加指令
subagent_list     查看任务
subagent_wait     等待完成
subagent_stop     停止任务
```

- `tools.go`：工具声明与调用转发。
- `types.go`：模型看到的参数形状。
- `plugin.go`：将这些工具登记到 `kernel/tools`。

任务状态、关系和通知逻辑都在 `kernel/subagents`；本目录不另存一份状态。
