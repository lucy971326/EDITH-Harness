# MCP tools

> 连接用户配置的 MCP Server，并把它们的工具填入 Harness 工具登记处。

```text
~/.harness/mcp.json
        ↓
读取 Server 配置 → 建立连接 → 获取工具 → 登记到 tools
```

- `config.go`：配置格式与校验。
- `provider.go`：连接、按 workspace 发现工具和调用。
- `result.go`：把 MCP 结果转换为 Harness 工具结果。
- `plugin.go`：解析依赖、登记 Provider、关闭连接。

这里不实现 MCP 工具本身，也不把远端数据写进 Session；Loop 会把实际调用与结果写账本。
