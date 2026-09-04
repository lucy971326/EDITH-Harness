# MCP 设计方向

## 一句话

MCP 是 Harness 的外部 Tool 来源；LLM 不直接调用 MCP，只调用转换后的普通 Tool。

```text
MCP Server
    │ 发现 / 调用
    ▼
MCP 插件 ──填充──▶ tools 登记处 ──普通 Tool──▶ Agent / Loop / LLM
```

MCP 插件放在 `plugins/kernel/tools/mcp`。Runner、Loop、Session 和 LLM 不需要知道 MCP 的存在。

## 两种范围

```text
用户级 MCP                         项目级 MCP
~/.harness/mcp.json               <workspace>/.mcp.json
                                  <workspace>/.harness/mcp.json
       │                                  │
       ▼                                  ▼
Agent 设置中手动选择                 随当前工作区自动启用
```

同名 Server 按以下顺序整项覆盖，不混合字段：

```text
用户级 < 项目根目录 < 项目 .harness 目录
```

不支持 `.agents/mcp.json`；`.agents` 继续用于 Skills。

## 两条读取路径

设置界面和运行时不能共用一份含义模糊的工具名单：

```text
Agent 设置
   └─ tools.Choices(ctx)
      └─ 内置 Tool + 用户级 MCP

Agent 运行
   └─ tools.ForWorkspace(ctx, workspace)
      └─ 项目级 MCP + 项目覆盖关系
```

本轮工具名单由两部分组成：

```text
Agent.Tools 中已选择的工具 + 当前 Workspace 自动工具
```

用户工具名保存进 `Agent.Tools`；项目工具不保存，始终根据本轮 Workspace 取得。

本轮的 Tool Definition、Server instructions 和实际 Call 必须来自同一份工作区快照，不能在调用中途重新读取配置。

## 基本原则

- 使用官方 MCP Go SDK，不自己实现协议。
- 配置采用 Claude Code 风格的 `mcpServers` JSON，`type` 必须明确。
- v1 支持 stdio 和 Streamable HTTP，不支持旧 SSE。
- MCP Tool 统一命名为 `mcp__<server>__<tool>`。
- Server 的整体 instructions 只在该 Server 有工具启用时交给 LLM。
- 工具目录在连接时发现并固定；配置或工具变化后重启 Harness。
- MCP 连接失败或工具定义无效时明确报错，不静默跳过。
- 项目 MCP 按工作区分别连接和保存，不能串到其他工作区。
- 用户级 MCP Tool 不自动加入默认 Agent；项目级 MCP Tool 自动加入当前工作区。

## v1 边界

v1 只接入 MCP Tools：发现 Tool、提供 Schema、调用 Tool、返回文本或结构化 JSON。

暂不支持 Resources、Prompts、Sampling、Elicitation、OAuth 和 MCP 专属审批。图片、音频及 Resource 结果返回明确的不支持错误。

普通文本结果超过 50 KiB 时截断；结构化 JSON 超限时返回错误，避免产生无效 JSON。

## 生命周期

用户级 MCP 连接由 MCP 插件持有；项目级 MCP 连接按规范化 Workspace 缓存。插件关闭时统一停止所有连接和 stdio 子进程。

MCP 只负责把外部工具接进 `tools` 登记处，不改变 Harness 的对话边界。
