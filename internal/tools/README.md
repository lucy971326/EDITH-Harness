# tools

普通工具与动态来源的统一调用入口。

```text
静态 Tool / MCP Provider -> Registry
Agent 选择 -> Prepare -> 本轮工具名单与来源快照
模型调用   -> Call -> 可用性 / 参数校验 -> PreToolUse -> 实际工具
```

- `types.go / tool.go`：工具契约、参数类型与处理函数。
- `registry.go`：登记、动态发现、Schema 查询和调用主线。
- `schema.go / access.go`：参数校验与可信访问上下文。
- `path.go / truncate.go`：工具共用的路径与输出处理。

Hook 明确拒绝会生成失败工具结果。具体审批、命令执行和文件修改交给 Tool 及其依赖；登记处不拥有进程或账本。

具体实现：[`exec`](exec/README.md)、[`applypatch`](applypatch/README.md)、[`mcp`](mcp/README.md)、[`subagents`](subagents/README.md)。实现依赖此包契约，由入口登记。

长期命令示例：exec_command 返回 process_id，后续 write_stdin 用它读取输出或继续输入。Tool 转换参数与结果，machine 持有进程／PTY／输出缓冲并校验 Session 归属；两者不各存一份进程状态。
