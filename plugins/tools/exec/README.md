# exec 工具

向模型提供 `exec_command` 和 `write_stdin`，共用 machine 的长期进程。

```text
exec_command -> 审批（需要时）-> AgentExec -> 输出 / process_id
write_stdin  -> AgentInteract -> 输入 / 后续输出
```

- `construct.go`：`Register` 登记两个工具。
- `exec_command.go / write_stdin.go`：参数、等待范围与调用。
- `output.go`：输出预算与模型可读结果。

进程状态在 machine，工具不建第二张进程表。调用身份由程序提供；已有进程沿用启动权限，`process_id` 不等于聊天 SessionID。
