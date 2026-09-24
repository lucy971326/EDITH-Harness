# machine

本机操作的接口与数据，全在 `types.go`；实现见 [`internal/machine/local`](local/README.md)。

```text
Agent Tool -> AgentProcesses / AgentFiles -> 携带可信 Policy
用户界面   -> FileSystem / TerminalSystem -> 直接操作
                         |
                    同一份 Local
```

- `AgentExec / AgentInteract`：启动和继续长期进程，已有进程沿用启动权限。
- `AgentApplyChanges`：批量提交带版本检查的文件修改。
- `FileSystem / PathSearcher`：文件、目录、监听与路径搜索。
- `TerminalSystem`：用户终端的 PTY 句柄。

依赖由构造函数传入。这里不操作系统、不审批，也不决定 Tool 是否对模型开放。
