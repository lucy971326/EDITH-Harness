# Hooks 系统：首版方向

## 范围

首版只实现 `PreToolUse` 命令 Hook，并在设置页提供 Hook 配置：启用状态、执行命令与参数、匹配的工具。暂不实现 `PermissionRequest`、`PostToolUse`、参数改写或 Hook 自动批准。Hook 不能绕过现有权限、审批和沙箱。

## 输入与决定

Harness 在工具执行前向 Hook 的 `stdin` 写入一份 JSON；首版字段为工具名、原始参数、调用 ID 和工作区，例如：

```json
{"tool_name":"exec_command","tool_input":{"cmd":"pwd"},"tool_call_id":"t1","workspace":"/project"}
```

Hook 的 `stdout` 为空表示不表态，继续原有流程；返回以下 JSON 表示拒绝本次调用：

```json
{"decision":"deny","reason":"不允许执行这条命令"}
```

Harness 解析决定；拒绝原因通过失败的工具结果交给模型，脚本的 stdout 不直接交给模型。

## 接入位置

在 `internal/tools/Registry.Call` 中，工具可用性与参数 Schema 校验之后、分发给静态工具或动态 Provider 之前执行 Hook。拒绝时不调用工具，返回 `IsError` 工具结果；现有 Loop 继续为该 tool call 落下一条配对的 tool result。没有匹配 Hook 时，调用路径不变。

## 实施前待定

- 设置的来源、持久化与项目 Hook 信任确认。
- 多个 Hook 的顺序、超时、取消、非零退出和无效输出的处理。
- 工具名匹配规则及设置页的具体交互。


# Harness `PreToolUse` Hook 首版

## 调研结论

交叉核查了 [OpenAI Docs](https://learn.chatgpt.com/docs/hooks) 和 Codex 源码：Codex 在工具处理器运行前调用 Hook；本地命令由**宿主进程**执行，明确拒绝才拦截工具，Hook 故障会报告并放行。它的信任哈希覆盖 Hook 配置，**不覆盖所引用脚本的内容**。Codex 使用 Shell 命令字符串；Harness 按你的选择使用 `command` 加 `args`。

## 实现方向

- 首版只做 `PreToolUse`。在 [Registry.Call](/Users/lucy/Documents/Projects/Harness/internal/tools/registry.go:193) 完成工具可用性及参数校验后、调用具体工具前，按**全局在前、项目在后**的顺序运行匹配 Hook；同一来源按设置顺序运行，遇到拒绝即停止。匹配只支持全部工具或精确工具名。
- 命令直接按 `command` 和 `args` 启动，不隐式经过 Shell；工作目录为当前工作区，继承宿主环境。`stdin` 使用[方向书](/Users/lucy/Documents/Projects/Harness/.forward/plan/hooks系统实现.md)已定的 JSON 字段。退出码为 0 且 `stdout` 为空表示不表态；退出码为 0 且输出 `{"decision":"deny","reason":"..."}` 表示拒绝，`reason` 必须非空。其他输出或非零退出均视为 Hook 失败。
- 每个 Hook 默认超时 10 秒，可在设置页配置 1–60 秒；限制输入和输出大小。超时、启动失败、无效输出时终止并等待 Hook 进程，**报告失败后继续原有工具流程**；用户停止 Run 时则终止 Hook，且不再执行工具。拒绝转换为配对的失败工具结果，原因交给模型；Hook 故障不交给模型。
- 全局配置存于 `~/.harness/hooks/settings.json`，项目配置存于 `<workspace>/.harness/hooks.json`。设置页可选择工作区，编辑两种来源的 Hook、顺序和启用状态，并查看最近失败。项目配置以真实工作区路径和**整份文件摘要**记录信任；页面保存即信任该版本，外部修改后项目 Hook 暂停运行，待页面重新确认。配置在**下一次工具调用**生效，已启动的 Hook 按原配置完成。全局 Hook 不受项目待信任状态影响。
- Hook 失败在当前运行界面显示简短提示，并在设置页保留最近原因；这些状态不进入会话账本。Go 与 TS 接口继续手写，不增加契约生成工具。

## 验证

重点验证静态工具和 MCP 工具都经过同一埋点；空输出、明确拒绝、多个 Hook 顺序、故障放行、Run 取消与工具结果配对。另验证项目配置外部变更后暂停、页面复信任、运行中停用在下一次调用生效，以及设置写入冲突不会覆盖外部修改。完成后运行 `make agent-check`；设置页由你运行并截图验收。

## 已接受的边界

按你的选择，**项目 Hook 也在宿主执行**，信任只绑定配置文件，不跟踪脚本内容。因此 Agent 若能改写项目脚本，后续 Hook 可能以宿主权限运行改写后的内容；设置页会明确提示这一点。
