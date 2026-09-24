# hooks

工具执行前运行用户配置的本地命令，目前只有 PreToolUse。

```text
Registry.Call -> Check -> 全局 Hook -> 已信任项目 Hook
                           -> command + args（宿主执行）
                           -> 空输出继续 / deny 阻止工具
```

- `types.go`：配置、设置页投影与保存参数。
- `service.go`：版本校验、项目信任、匹配顺序和命令执行。
- `process_unix.go / process_windows.go`：平台进程取消与清理。

stdin 是含 `tool_name / tool_input / tool_call_id / workspace` 的 JSON。退出码 0 且 stdout 为空表示不表态；拒绝需输出 `{"decision":"deny","reason":"原因"}`。故障提示后放行，Run 取消则停止，不继续执行工具。

全局配置在 `~/.harness/hooks/settings.json`，项目配置在 `.harness/hooks.json`。项目按真实路径与配置文件摘要信任，**不跟踪脚本内容**；保存检查版本，下一次工具调用读取新配置。匹配支持全部或精确工具名，默认超时 10 秒，可设 1–60 秒。

## 决策边界

- Hook 在参数校验后、静态工具或 MCP 分发前运行，不能绕过工具权限、审批或沙箱；目前不做自动批准、参数改写或其他事件。
- 同一来源按设置顺序执行，全局先于项目；遇拒绝停止后续 Hook。命令直接使用 command + args，不隐式经过 Shell；工作目录为当前工作区，继承宿主环境。
- 非零退出、无效输出、超时或启动失败属于故障；reason 必须非空，stdout 不直接交给模型。拒绝转换为配对失败工具结果交给模型，故障只报告给用户。
- 输入输出有大小上限；超时／取消终止并等待进程清理。项目配置外部修改暂停项目 Hook，不影响全局；页面保存即信任该版本，保存冲突不能覆盖外部修改。
- 配置下一次调用生效，在途 Hook 使用原配置。脚本内容不在信任摘要内，是明确接受的边界；修改脚本可改变后续宿主执行行为。
