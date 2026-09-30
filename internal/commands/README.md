# commands

提供两种命令：启动时登记、立即执行的平台命令，以及保存在用户或工作区配置中的提示词命令。

```text
入口 Register(compact)
  -> Registry
  <- conversations.CallCommand -> Get -> 命令.Run

~/.harness/commands/settings.json + <workspace>/.harness/commands.json
  -> PromptStore -> command/list 并列展示来源
  -> 会话 Send(commandID) -> 读取最新模板并展开 -> Runner
```

`types.go` 定义命令契约，`registry.go` 登记平台命令，`prompts.go` 校验和读写本地模板。相同名称的用户与工作区命令用来源 ID 区分；未选择来源的同名调用由 Client 提示选择。`/` 是 Client 的输入交互；压缩业务属于 Runner，不在登记处实现。提示词命令发送时才由后端展开，原调用文本另存用于显示；模板内容不作为用户直接授权。

当前实现为 [`compact`](compact/README.md)，由入口构造并登记。
