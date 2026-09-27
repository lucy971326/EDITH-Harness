# MCP tools

连接 MCP Server，把远端工具适配成统一 Tool 来源。

```text
全局 mcp.json + 项目配置（需信任）
  -> Provider -> 工具目录 -> tools.Registry -> Provider.Call
```

- `construct.go`：`New` 创建 Provider，不在启动时连接 Server。
- `config.go`：配置解析与展开。
- `settings.go`：全局配置版本保护、读写与安全状态视图；项目配置只读。
- `runtime.go`：按 Run 固定连接、项目信任、失败隔离和释放。
- `provider.go`：MCP 连接、工具发现与快照组装。
- `oauth_http.go`、`oauth_flow.go`：安全发现、显式登录任务及本机回调。
- `oauth_store.go`、`oauth_credential.go`：作用域凭据、令牌刷新及普通 Run 的只读授权状态。
- `result.go`：MCP 结果转成 Harness 工具结果。

项目来源是 `.mcp.json` 与 `.harness/mcp.json`。只读模式禁用 MCP；其他模式按规则确认配置、审核调用。Server 在宿主或远端运行，不自动进入 Agent 命令沙箱。入口负责 Close；结果经 Loop 交给 Runner 落账。
