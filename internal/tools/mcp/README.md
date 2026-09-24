# MCP tools

连接 MCP Server，把远端工具适配成统一 Tool 来源。

```text
全局 mcp.json + 项目配置（需信任）
  -> Provider -> 工具目录 -> tools.Registry -> Provider.Call
```

- `construct.go`：`New` 读取全局配置并创建 Provider。
- `config.go`：配置解析、合并与校验。
- `provider.go`：工作区发现、配置快照、连接、调用和 Close。
- `result.go`：MCP 结果转成 Harness 工具结果。

项目来源是 `.mcp.json` 与 `.harness/mcp.json`。只读模式禁用 MCP；其他模式按规则确认配置、审核调用。Server 在宿主或远端运行，不自动进入 Agent 命令沙箱。入口负责 Close；结果经 Loop 交给 Runner 落账。
