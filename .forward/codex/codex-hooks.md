# Codex Hooks

本地参考源码定义了 **12 种 Hook 事件**：[事件清单](../../reference/codex/codex-rs/hooks/src/lib.rs#L24)。

| Hook | 触发时机与作用 |
| --- | --- |
| `PreToolUse` | 工具执行前：检查、阻止或修改参数 |
| `PermissionRequest` | 需要审批时：自定义审核可返回批准、拒绝或不作决定 |
| `PostToolUse` | 工具成功执行后：处理返回结果、补充反馈，不能撤销已发生的操作 |
| `UserPromptSubmit` | 用户提交消息时 |
| `SessionStart` | 会话启动、恢复等时机 |
| `SessionEnd` | 会话结束时 |
| `PreCompact` | 压缩对话上下文前 |
| `PostCompact` | 压缩对话上下文后 |
| `SubagentStart` | 子 Agent 启动时 |
| `SubagentStop` | 子 Agent 准备结束本轮工作时 |
| `Stop` | Agent 准备结束本轮回复时，可要求继续检查 |
| `Interrupt` | 本轮运行被中断时，例如用户点击停止 |

审批流程：`需要审批 → PermissionRequest Hook → 若未决定，交给内置自动审核或用户`。没有配置 Hook 时，内置审批照常工作。[审批入口](../../reference/codex/codex-rs/core/src/tools/approvals.rs#L474)

接入方式：统一 Tool 入口调用前后 Hook，审批入口调用审批 Hook；匹配与执行逻辑集中在独立的 `hooks` 模块。
