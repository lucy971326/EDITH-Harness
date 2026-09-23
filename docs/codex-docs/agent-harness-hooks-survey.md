# Agent Harness Hooks Survey

查阅日期：2026-09-23。本篇记录其他产品的 Hook 触发点与处理方式，供后续调研使用；不代表 Harness 已决定实现这些能力。Harness 当前方向见 [overview.md](../../.forward/plan/overview.md)。

| 产品 | 触发点 | 实际支持的处理方式 | 依据 |
| --- | ---: | --- | --- |
| Claude Code | 33 种 | `command`、`prompt`、`agent`、`http`、`mcp_tool` | [官方 Hooks reference](https://code.claude.com/docs/en/hooks) |
| Codex | 12 种 | `command`、`mcp_tool` | [OpenAI Docs：Hooks](https://learn.chatgpt.com/docs/hooks) |
| Antigravity | 5 种 | `command` | 用户提供的 `hooks.md:1-331` 摘要；未独立核验原文 |
| Grok Build | 15 种 | `command`、`http` | [公开文档](https://x.ai/docs/build/features/hooks)；本机 `~/.grok/docs/user-guide/10-hooks.md`（含 `StopCancelled`） |

## Claude Code

| 场景 | 触发点 |
| --- | --- |
| 工具与权限 | `PreToolUse`、`PermissionRequest`、`PermissionDenied`、`PostToolUse`、`PostToolUseFailure`、`PostToolBatch` |
| 用户输入与消息 | `UserPromptSubmit`、`UserPromptExpansion`、`MessageDisplay` |
| 会话与回答 | `Setup`、`SessionStart`、`SessionEnd`、`Stop`、`StopFailure`、`Notification` |
| 子 Agent 与任务 | `SubagentStart`、`SubagentStop`、`TeammateIdle`、`TaskCreated`、`TaskCompleted` |
| 上下文与模型 | `PreCompact`、`PostCompact`、`PreModelSwitch`、`PostModelSwitch` |
| 配置与工作区 | `ConfigChange`、`InstructionsLoaded`、`CwdChanged`、`DirectoryAdded`、`FileChanged`、`WorktreeCreate`、`WorktreeRemove` |
| MCP 用户输入 | `Elicitation`、`ElicitationResult` |

- `command` 执行本机命令；`http` 把事件 JSON POST 到 URL；`mcp_tool` 调用已连接 Server 的工具；`prompt` 做单次模型判断；`agent` 启动可用工具调查的子 Agent。
- 处理方式与触发点并非任意组合：例如 `PermissionRequest` 不支持 `agent`；`PostToolUse` 与 `PostToolUseFailure` 分别对应成功与失败。

## Codex

| 场景 | 触发点 |
| --- | --- |
| 工具与审批 | `PreToolUse`、`PermissionRequest`、`PostToolUse` |
| 用户输入与一轮结束 | `UserPromptSubmit`、`Stop`、`Interrupt` |
| 上下文压缩 | `PreCompact`、`PostCompact` |
| 会话与子 Agent | `SessionStart`、`SessionEnd`、`SubagentStart`、`SubagentStop` |

- `command` 执行脚本或命令；`mcp_tool` 调用已经连接的 MCP Server 工具。`prompt` 和 `agent` 配置可被解析，但当前运行时跳过；官方文档未列出 HTTP 处理器。
- `Interrupt` 和 `SessionEnd` 不在子 Agent 上触发；`mcp_tool` 不支持 `SessionEnd`，也不会自行建立或重连 MCP 连接。
- 非托管 Hook 需按当前定义经过用户审查与信任，定义变更后重新审查。

## Antigravity

以下仅依据用户提供的 `hooks.md:1-331` 摘要，尚未对照原文或实机验证。

| 触发点 | 时机与主要能力 |
| --- | --- |
| `PreToolUse` | 工具执行前；可允许、拒绝、询问或改写参数 |
| `PostToolUse` | 工具执行后；用于校验、清理或收集诊断 |
| `PreInvocation` | 模型调用前；可注入临时上下文或预置步骤 |
| `PostInvocation` | 模型输出且工具执行完成后；可控制继续或结束 |
| `Stop` | 执行循环准备结束时；可要求继续 |

- 处理器仅有 `command`：宿主 Shell 执行脚本或命令，通过 `stdin` 接收 CamelCase JSON，通过 `stdout` 返回 JSON 决定。
- 摘要称执行为同步阻塞，可设置超时，默认 30 秒。

## Grok Build

以下以本机 `~/.grok/docs/user-guide/10-hooks.md` 为准；用户提供的[公开文档](https://x.ai/docs/build/features/hooks)列出 14 个事件，本机指南增加 `StopCancelled`。公开页面此次未能直接读取，差异数量依据用户调研。

| 场景 | 触发点与作用 |
| --- | --- |
| 会话 | `SessionStart`、`SessionEnd`：观察会话开始、结束；前者可用于初始化 |
| 输入 | `UserPromptSubmit`：可拒绝用户提交的提示；自动唤醒、定时任务和子代理会话只观察 |
| 工具与权限 | `PreToolUse`：可拒绝、要求询问、暂不决定或改写工具参数；`PostToolUse`：可给模型补充说明或替换模型看到的结果；`PostToolUseFailure`：可补充上下文；`PermissionDenied`：只观察 |
| 一轮结束 | `Stop`、`SubagentStop`：可要求继续或强制结束；`StopFailure`、`StopCancelled`：分别观察 API 错误和未完成的取消 |
| 其他 | `Notification`、`SubagentStart`、`PreCompact`、`PostCompact`：只观察 |

- `command` 跑本地命令，事件 JSON 从 `stdin` 输入；`http` 把同一事件 JSON POST 到 URL。可在个人 `~/.grok/hooks/*.json` 或项目 `.grok/hooks/*.json` 配置；项目 Hook 需通过 `/hooks-trust` 或 `--trust` 信任。
- `matcher` 用正则选择工具名、通知类型、子代理类型等，省略则全匹配。Grok 也读取 Claude 与 Cursor 的 Hook 配置；Cursor 事件名映射为上述事件，Claude 工具名在匹配时映射为 Grok 工具名，例如 `Bash` → `run_terminal_command`、`Read` → `read_file`、`Edit` / `Write` → `search_replace`。
- `PreToolUse` 可返回 `deny`、`ask`、`defer` 或 `updatedInput`；`PostToolUse` 已无法阻止执行，但可通过 `updatedToolOutput` 改变模型看到的结果，原始结果仍保留。`Stop` / `SubagentStop` 最多让同一轮继续 8 次。
- Hook 超时、崩溃或返回无效 JSON 时通常放行并记录失败；例外是 `updatedInput` 不符合工具 Schema 时会阻止该调用。通常默认超时 5 秒，`Stop` / `SubagentStop` / `PostToolUse` 默认 600 秒；本机指南另列 `UserPromptSubmit` 默认 30 秒。
- `[[ui.notifications.hooks]]` 是独立的终端通知机制，可设置为仅在终端失焦时运行提醒命令；其 5 个事件是 `turn_complete`、`approval_required`、`session_ready`、`task_complete`、`agent_error`，不计入上述 15 个生命周期事件。依据本机 `~/.grok/docs/user-guide/05-configuration.md`。
