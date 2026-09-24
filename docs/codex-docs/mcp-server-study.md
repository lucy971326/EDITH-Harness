# Codex 的三种服务器角色

本篇压缩自此前对本地 reference/codex 的研究，不代表最新上游状态，也不是 Harness 架构规范。Client 通信细节以[专项笔记](client-communication.md)为准。

| 角色 | 谁调用 | 负责什么 |
| --- | --- | --- |
| app-server | UI / Client | 会话、Turn、审批与运行通知 |
| exec-server | Codex 的执行客户端 | 进程、文件与执行环境能力；不是 MCP |
| TUI DynamicToolMcpServer | 外部 MCP Client | 把特定工具以 MCP 协议暴露出去 |

```text
UI → app-server → core → 执行后端
                   └→ 外部 MCP Server

外部 MCP Client → TUI DynamicToolMcpServer → 暴露的工具
```

## 值得记住的区别

- exec-server 的 process/start、read、write、signal 等属于机器执行协议；文件与进程生命周期归执行方。
- app-server 是 UI 接入面，不是 exec-server 的别名；是否通过独立执行服务要看具体执行路径，不能把所有工具都画成固定远程调用。
- “Codex 调外部 MCP”与“Codex 自己暴露 MCP Server”方向相反；后者还要区分 TUI 动态工具服务与其他 MCP 入口。
- 当时研究的 exec-server 提供 stdio / WebSocket，TUI 动态 MCP 使用本地 streamable HTTP。不要由名字推断传输方式，使用时查对应入口。
- 这些边界解释职责，不要求 Harness 增加三套服务。Harness 当前通过进程内 machine 调用本机能力。

## 源码定位

在 reference/codex/codex-rs 中按以下符号定位，以当前源码为准：

- CLI 的 ExecServerSubcommand：独立执行服务启动入口。
- exec-server / exec-server-client：执行服务及调用方。
- DynamicToolMcpServer：TUI 动态 MCP 工具暴露。
- app-server：Client 接入与请求分发。

原文逐方法清单、完整源码摘抄与重复调用图已移除；需要实现细节时直接读源码。
