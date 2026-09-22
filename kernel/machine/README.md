# machine

【它是什么】操作本机的公共契约，实现位于 `plugins/machine/local`。

```text
Agent Tool → AgentProcesses / AgentFiles → 按可信 Policy 执行
用户界面   → FileSystem / TerminalSystem → 用户直接操作
                       ↓
                同一份 machine 服务
```

- `AgentExec`：携带权限启动命令。
- `AgentInteract`：继续操作已有进程，不能改变启动权限。
- `AgentApplyChanges`：携带权限提交一批版本受保护的文件修改。
- `FileSystem`、`PathSearcher`、`TerminalSystem`：文件、搜索、监听与用户终端。

这里只定义接口与数据，不直接操作系统。Tool 从 Host 取得服务，不 import 本机实现；实际接入状态见 [STATUS.md](../../STATUS.md)。
