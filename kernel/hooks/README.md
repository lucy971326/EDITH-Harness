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
