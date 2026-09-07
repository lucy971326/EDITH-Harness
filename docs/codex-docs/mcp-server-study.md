# Codex 的"服务器"机制：exec-server / MCP Server / app-server 完全搞懂

> 基于 `reference/codex/codex-rs/` 源码的真实行为整理。

---

## 1. 概览：三个"服务器"

Codex 里有**三种不同的"服务器"角色**，职责完全不同：

```
┌─────────────────────────────────────────────────────────────────┐
│                    Codex 里的三种"服务器"                          │
├──────────────────┬──────────────────┬────────────────────────────┤
│   exec-server    │   MCP Server     │   app-server               │
│  (JSON-RPC)      │  (MCP Protocol)  │  (类 JSON-RPC)             │
├──────────────────┼──────────────────┼────────────────────────────┤
│ 命令/文件执行    │  外部工具暴露     │  UI 通信网关               │
│ 沙箱隔离         │  AI 模型调用工具   │  线程/Turn 生命周期        │
│ 独立的子进程     │  本地 HTTP       │  150+ 请求方法              │
│ stdio / WebSocket│  HTTP (streamable)│  ws/stdio/uds/remote       │
└──────────────────┴──────────────────┴────────────────────────────┘
```

---

## 2. exec-server：真正的"stdio 服务器"

### 2.1 它是什么

`exec-server` 是 Codex 的**执行后端**——一个独立的二进制，以**子进程**或**远程服务**的形式运行。

它监听 **stdio** 或 **WebSocket**，使用**自定义 JSON-RPC 协议**（不是 MCP 协议）。

启动方式：

```bash
# CLI 子命令（experimental）
codex exec-server --listen stdio
codex exec-server --listen ws://127.0.0.1:0

# 远程注册（支持 Noise / Direct 传输，AWS SigV4 签名）
codex exec-server --remote <URL> --environment-id <ID> \
  [--remote-transport Noise|Direct] \
  [--use-agent-identity-auth] \
  [--aws-sigv4 --aws-profile <PROFILE> --aws-region <REGION>]
```

exec-server CLI 子命令定义在 `cli/src/main.rs`（`ExecServerSubcommand` 枚举），help text: `[EXPERIMENTAL] Run the standalone exec-server service.`

### 2.2 它暴露的"工具"（27 个协议方法常量）

```
process/start          — 执行命令（核心方法）
process/read           — 读命令 stdout
process/write          — 写命令 stdin
process/signal         — 发信号（SIGINT 等）
process/terminate      — 终止进程
process/output         — 输出增量通知（服务端推送）
process/exited         — 进程退出通知（服务端推送）
process/closed         — 进程关闭通知（服务端推送）

environment/info       — 环境信息
environment/status     — 环境状态
environmentConfig/read — 读取环境配置
capabilityRoots/discoverV1 — 发现能力根目录

fs/readFile            — 读文件
fs/open                — 打开文件
fs/readBlock           — 读文件块
fs/close               — 关闭文件
fs/writeFile           — 写文件
fs/createDirectory     — 创建目录
fs/getMetadata         — 获取文件元数据
fs/canonicalize        — 规范化路径
fs/readDirectory       — 读目录内容
fs/walk                — 遍历目录树
fs/remove              — 删除文件/目录
fs/copy                — 复制文件/目录

http/request           — 发送 HTTP 请求
http/request/bodyDelta — HTTP 请求体增量

network/policyRequest  — 网络策略请求（沙箱层处理）
network/policyDecision — 网络策略决策（沙箱层处理）
```

路由器实际注册了 **24 个条目**（1 个通知 + 23 个请求）。以下方法常量**不在主路由器中**：
- `process/output`、`process/exited`、`process/closed`：服务端主动推送的通知
- `network/policyRequest`、`network/policyDecision`：在沙箱/进程层单独处理

### 2.3 协议细节

**不是 MCP 协议。** exec-server 使用自己的 JSON-RPC dialect（"Codex JSON-RPC"）：

- 协议文件：`exec-server-protocol/src/protocol.rs`（27 个方法常量）
- 类型定义：`exec-server-protocol/src/rpc.rs`
- 连接处理：`exec-server/src/connection.rs`（`JsonRpcConnection`）
- 路由器：`exec-server/src/server/registry.rs`（`RpcRouter<ExecServerHandler>`）

**关键差异：** 标准 JSON-RPC 2.0 在每条消息上携带 `"jsonrpc": "2.0"` 字段。exec-server 的 Codex JSON-RPC dialect 省略了该字段，只保留 `id`/`method`/`params`（请求）和 `id`/`result`（响应）。

```rust
// exec-server-protocol/src/rpc.rs 模块注释:
// "Exec-server uses the Codex JSON-RPC dialect, which omits the
//  \"jsonrpc\": \"2.0\" field on the wire."
//
// JSONRPCRequest 结构体没有 jsonrpc 字段:
// pub struct JSONRPCRequest {
//     pub id: RequestId,
//     pub method: String,
//     pub params: Option<serde_json::Value>,
//     pub trace: Option<W3cTraceContext>,
// }
```

### 2.4 stdio 模式的关键代码

```rust
// exec-server/src/server/transport.rs
ExecServerListenTransport::Stdio => {
    run_stdio_connection(
        runtime_paths,
        telemetry,
        http_client_factory,
        request_dispatch_mode,
    )
    .await
}

async fn run_stdio_connection(...) {
    run_stdio_connection_with_io(
        io::stdin(),   // ← 从 stdin 读 JSON-RPC
        io::stdout(),  // ← 向 stdout 写 JSON-RPC
        ...
    ).await
}

// 每个 stdio 连接只服务一个请求
// "Stdio serves exactly one connection"
```

### 2.5 WebSocket 安全

exec-server 的 WebSocket 传输使用中间件拒绝带 `Origin` 头的请求：

```rust
// exec-server/src/server/transport.rs
async fn reject_requests_with_origin_header(
    request: Request<Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    if request.headers().contains_key(ORIGIN) {
        warn!(...);
        Err(StatusCode::FORBIDDEN)
    } else {
        Ok(next.run(request).await)
    }
}
```

这是一个安全措施——阻止跨域 WebSocket 劫持攻击。

---

## 3. MCP Server：TUI 的 DynamicToolMcpServer

### 3.1 它是什么

TUI 可以暴露一个**本地 MCP 服务器**，让外部 MCP 客户端（如 Claude Desktop、其他 AI 工具）通过 MCP 协议控制 Codex 实例。

关键代码：`tui/src/dynamic_tools_mcp.rs`（324 行）

### 3.2 它暴露的 MCP 工具

| MCP 工具 | 含义 |
|---|---|
| `create_thread` | 创建新对话 |
| `send_message_to_thread` | 向对话发消息 |
| `fork_thread` | Fork 对话 |
| `read_thread` | 读对话内容 |
| `list_threads` | 列出对话 |
| `list_archived_threads` | 列出归档对话 |
| `wait_threads` | 等待对话状态 |
| `set_thread_title` | 设置对话标题 |
| `set_thread_archived` | 设置对话归档状态 |

### 3.3 传输方式：HTTP（streamable HTTP），不是 stdio

```rust
// tui/src/dynamic_tools_mcp.rs
impl DynamicToolMcpServer {
    pub(crate) async fn start(...) -> std::io::Result<Self> {
        let listener = TcpListener::bind("127.0.0.1:0").await?;  // ← 随机端口
        let address = listener.local_addr()?;
        let authorization = Arc::new(format!("Bearer {}", Uuid::new_v4()));

        // 生成 MCP 服务器配置（包含 URL 和认证头）
        let server_config = json!({
            "url": format!("http://{address}/mcp"),
            "http_headers": {"Authorization": authorization.as_str()},
            "default_tools_approval_mode": "approve",
            "tools": { ... }
        });

        // 使用 rmcp 的 StreamableHttpService
        let service = StreamableHttpService::new(
            move || Ok(handler.clone()),
            Arc::new(LocalSessionManager::default()),
            StreamableHttpServerConfig::default(),
        );

        let router = Router::new()
            .nest_service("/mcp", service)
            .layer(middleware::from_fn_with_state(
                authorization,
                require_authorization,
            ));

        let task = tokio::spawn(async move {
            axum::serve(listener, router).await
        });
    }
}
```

**关键点：**
- 使用 `rmcp::handler::server::ServerHandler` trait
- 使用 `rmcp::transport::StreamableHttpService`
- 监听 `127.0.0.1:0`（随机端口）
- Bearer Token 认证
- **不是 stdio 传输**

### 3.4 怎么用

TUI 启动时，如果启用了 `Dynamic` 或 `Mcp` 模式，会自动启动这个本地 MCP 服务器。外部客户端可以通过 MCP 协议连接：

```json
{
  "mcpServers": {
    "codex-tui": {
      "url": "http://127.0.0.1:RANDOM_PORT/mcp",
      "http_headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

---

## 4. app-server：UI 通信网关

### 4.1 它是什么

app-server 是 Codex 引擎的**服务端壳子**。所有 UI（TUI / CLI / VSCode / Desktop / Web）都通过 app-server 跟 core 交互。

它是**唯一直接持有 codex-core 实例**的组件（生产环境中）。

### 4.2 它暴露的协议

类 JSON-RPC 协议（150+ 请求方法，来自 39 个处理器文件），包括：

**线程管理（36 个操作）：**
- `thread/start`, `thread/resume`, `thread/fork`
- `thread/archive`, `thread/delete`, `thread/compact`
- `thread/turns/list`, `thread/items/list`
- `thread/search`, `thread/sections`

**Turn 执行（13 个操作）：**
- `turn/start`, `turn/interrupt`, `turn/steer`
- `thread/realtime/start`, `thread/realtime/stop`

**审批中转：**
- `exec/approval`, `fileChange/approval`, `permissions/approval`

**MCP 管理（8 个操作）：**
- `mcpServer/oauth/login` — OAuth 登录
- `config/mcpServer/reload` — 重新加载 MCP 配置
- `mcpServerStatus/list` — 列出 MCP 服务器状态
- `mcpServer/resource/read` — 读取 MCP 资源
- `mcpServer/event/stream/start` — [实验性] 启动 MCP 事件流
- `mcpServer/event/stream/stop` — [实验性] 停止 MCP 事件流
- `mcpServer/tool/call` — 调用 MCP 工具
- `mcpServer/elicitation/request` — MCP 服务器请求用户输入（服务端推送）

**配置/诊断：**
- `config/read`, `config/write`, `diagnostics`

### 4.3 传输方式

```
ws://IP:PORT   — WebSocket（远程/多客户端）
stdio://        — Stdio（默认，独立 app-server 进程）
unix://PATH     — Unix Domain Socket（本机其他客户端）
off             — 禁用所有监听
```

---

## 5. exec-server vs MCP Server vs app-server 对比

### 5.1 定位差异

```
┌──────────────────────────────────────────────────────────────────┐
│   exec-server                                                    │
│   ┌────────────────────────────────────────────────────────┐    │
│   │ 角色：执行后端 (Execution Backend)                       │    │
│   │ 协议：自定义 JSON-RPC (不是 MCP)                          │    │
│   │ 传输：stdio / WebSocket                                   │    │
│   │ 生命周期：被 app-server 作为子进程启动，或远程独立运行      │    │
│   │ 工具：20 个方法（process/*, fs/*, http/*, environment/*）  │    │
│   │ 隔离：沙箱执行（seatbelt/landlock/sandbox）               │    │
│   └────────────────────────────────────────────────────────┘    │
│                          ▲                                       │
│                    调用关系                                      │
│                          │                                       │
│   ┌────────────────────────────────────────────────────────┐    │
│   │ app-server                                               │    │
│   │ 角色：协议网关 (Protocol Gateway)                         │    │
│   │ 协议：类 JSON-RPC (非严格 JSON-RPC 2.0)                    │    │
│   │ 传输：ws / stdio / unix / remote                          │    │
│   │ 生命周期：长期运行，持有 core 实例                          │    │
│   │ 工具：46 个请求处理器（thread/*, turn/*, mcpServer/* 等）  │    │
│   │ 职责：线程管理 / Turn 执行 / 审批中转 / 用户输入            │    │
│   └────────────────────────────────────────────────────────┘    │
│                          ▲                                       │
│                    调用关系                                      │
│                          │                                       │
│   ┌────────────────────────────────────────────────────────┐    │
│   │ UI (TUI / CLI / VSCode / Desktop)                        │    │
│   │ 角色：用户界面                                             │    │
│   │ 通信：通过 app-server 的类 JSON-RPC 协议                    │    │
│   └────────────────────────────────────────────────────────┘    │
│                                                                  │
│   ┌────────────────────────────────────────────────────────┐    │
│   │ MCP Server (TUI 的 DynamicToolMcpServer)                 │    │
│   │ 角色：外部 MCP 客户端接入点                                │    │
│   │ 协议：标准 MCP (rmcp ServerHandler)                       │    │
│   │ 传输：HTTP streamable (127.0.0.1:0)                       │    │
│   │ 生命周期：TUI 启动时创建，TUI 退出时销毁                     │    │
│   │ 工具：7 个 MCP 工具（thread 操作）                         │    │
│   └────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
```

### 5.2 详细对比表

| 维度 | exec-server | MCP Server (TUI) | app-server |
|------|-------------|-------------------|------------|
| **角色** | 执行后端 | 外部 MCP 接入 | UI 协议网关 |
| **协议** | 自定义 JSON-RPC | 标准 MCP | 类 JSON-RPC |
| **传输** | stdio / WebSocket | HTTP streamable | ws/stdio/unix/remote |
| **启动方式** | 子进程 / 远程独立 | TUI 自动启动 | 独立进程 / Embedded |
| **生命周期** | 按需创建/销毁 | TUI 生命周期 | 长期运行 |
| **持有 core** | 否 | 否 | **是** |
| **工具数量** | 24 个请求处理器 | 9 个 MCP 工具 | 150+ 请求处理器 |
| **沙箱** | 有（seatbelt/landlock） | 无 | 无（委托 exec-server） |
| **认证** | 无 / 环境隔离 | Bearer Token | WebSocket auth / 无 |

---

## 6. exec-server 与 app-server 的关系

### 6.1 调用链

```
UI → app-server → exec-server
          │            │
          │            └── 执行命令 / 读写文件 / HTTP 请求
          │
          └── 管理线程 / Turn / 审批
```

app-server **不直接** 启动 exec-server。实际流程是：

1. app-server 创建 `EnvironmentManager::from_codex_home()`（`app-server/src/lib.rs`）
2. `EnvironmentManager` 加载 `environments.toml` 配置
3. 当需要 stdio 连接时，exec-server 客户端库（`exec-server/src/client_transport.rs`）的 `connect_stdio_command` 负责生成子进程

app-server 通过 `EnvironmentManager` 间接管理 exec-server 生命周期，不持有子进程句柄。

### 6.2 为什么分离

```
exec-server 独立出来的原因：
├── 沙箱隔离：exec-server 在受限环境中运行
├── 安全：命令执行和文件操作需要独立的安全边界
├── 远程执行：exec-server 可以作为远程服务独立部署
└── 进程隔离：crash 不会影响 app-server
```

---

## 7. MCP Server 与 app-server 的关系

### 7.1 职责边界

```
app-server 的 MCP 相关职责：
├── 作为 MCP 请求转发层（不是 MCP 客户端本身）
│   ├── McpRequestProcessor 接收 UI 发来的 MCP 相关请求
│   ├── 转发给 codex-core 的 McpManager 执行
│   └── McpManager 负责管理 MCP 服务器配置和工具调用
│
├── 管理 MCP 服务器的生命周期
│   ├── 启动 / 重连 / 断开
│   ├── OAuth 认证
│   └── 工具列表缓存
│
├── 将 MCP 工具暴露给 AI 模型
│   ├── 工具调用路由到对应的 MCP 服务器
│   ├── 审批中转（exec approval / file change approval）
│   └── 工具结果返回给模型
│
└── 向 UI 暴露 MCP 状态
    ├── mcpServerStatus/list
    ├── mcpServer/tool/call
    └── mcpServer/oauth/login
```

**注意：** 真正的 MCP 客户端逻辑在 `codex_core::McpManager`（`core/src/mcp.rs`），使用 `codex_rmcp_client` crate 与外部 MCP 服务器通信。app-server 只是请求转发层（`McpRequestProcessor` 接收 UI 请求，转发给 `ThreadManager::call_mcp_tool()` 执行）。

### 7.2 MCP Server (TUI) 与 app-server 的关系

```
外部 MCP 客户端 → TUI 的 DynamicToolMcpServer (HTTP)
                              │
                              ▼
                       app-server (Embedded)
                              │
                              ▼
                           codex-core
```

TUI 的 MCP Server 是一个**反向接入点**——让外部工具能控制 Codex，而不是 Codex 调用外部工具。

---

## 8. 完整架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         外部 MCP 客户端                               │
│                  (Claude Desktop / 其他 AI 工具)                      │
└────────────────────────────┬────────────────────────────────────────┘
                             │ MCP Protocol (HTTP)
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│  TUI 的 DynamicToolMcpServer (MCP Server)                           │
│  • 7 个 MCP 工具（thread 操作）                                       │
│  • 监听 127.0.0.1:0（随机端口）                                       │
│  • Bearer Token 认证                                                  │
│  • 调用 app-server 的 Embedded 接口                                  │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  UI 层                                                                │
│  ┌──────────┬──────────┬──────────┬──────────┐                      │
│  │   TUI    │   CLI    │  VSCode  │ Desktop  │                      │
│  └──────────┴──────────┴──────────┴──────────┘                      │
└────────────────────────────────────┬─────────────────────────────────┘
                                     │ 类 JSON-RPC
                                     ▼
┌─────────────────────────────────────────────────────────────────────┐
│  app-server (协议网关)                                                │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 150+ 请求处理器（协议方法）                                      │  │
│  │ • 线程管理 (thread_processor.rs, 36 个操作)                     │  │
│  │ • Turn 执行 (turn_processor.rs, 13 个操作)                     │  │
│  │ • 审批中转 (bespoke_event_handling.rs)                          │  │
│  │ • MCP 请求转发 (mcp_processor.rs)                              │  │
│  │ • 配置读写 (config_processor.rs)                               │  │
│  │ • 插件管理 (plugins.rs)                                        │  │
│  │ • 账户管理 (account_processor.rs)                              │  │
│  │ • ... 共 39 个处理器文件                                        │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│                    调用 exec-server                                   │
│                              │                                        │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ exec-server (子进程或远程)                                       │  │
│  │ • 自定义 JSON-RPC 协议（不是 MCP）                               │  │
│  │ • 24 个请求处理器（process/*, fs/*, http/*, environment/*）     │  │
│  │ • stdio / WebSocket 传输                                        │  │
│  │ • 沙箱隔离（seatbelt / landlock / Windows sandbox）             │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│  codex-core                                                         │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ MCP Client (rmcp-client / McpManager)                          │  │
│  │ • 连接外部 MCP 服务器（stdio / streamable-http / SSE）          │  │
│  │ • 管理 MCP 服务器生命周期                                        │  │
│  │ • OAuth 认证                                                    │  │
│  └───────────────────────────────────────────────────────────────┘  │
│  • Session / Task / Turn 循环                                        │
│  • 模型 API 调用                                                     │
│  • 工具执行编排                                                      │
│  • rollout 持久化（codex-rollout crate）                             │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 9. 数据流示例

### 9.1 UI → app-server → exec-server（执行命令）

```
TUI → app-server: thread/turn/start (prompt: "ls -la")
app-server → core: 创建 Turn
core → app-server: 需要执行 "ls -la"
app-server → exec-server: process/start (command: "ls -la")
exec-server → app-server: process/output (stdout: "total 0...")
app-server → core: 命令输出
core → app-server: agent_message (包含输出)
app-server → TUI: agent_message 推送
```

### 9.2 外部 MCP 客户端 → TUI 的 MCP Server

```
Claude Desktop → TUI DynamicToolMcpServer: tools/call (list_threads)
TUI → app-server (Embedded): thread_list
app-server → TUI: thread_list_response
TUI → Claude Desktop: CallToolResult (线程列表)
```

### 9.3 app-server → 外部 MCP 服务器

```
app-server → 外部 MCP 服务器: tools/list
外部 MCP 服务器 → app-server: [tools...]
app-server → core: 工具列表
core → 模型: 可用工具上下文
模型 → app-server: tool_call (github_create_issue)
app-server → 外部 MCP 服务器: tools/call (github_create_issue)
外部 MCP 服务器 → app-server: 结果
```

---

## 10. 总结

### 核心区别

```
exec-server  = 执行引擎（沙箱里的命令/文件/HTTP 执行器）
               协议：自定义 JSON-RPC
               传输：stdio / WebSocket
               角色：app-server 的"手脚"

MCP Server   = 外部接入点（让其他 AI 工具能调用 Codex）
               协议：标准 MCP
               传输：HTTP streamable
               角色：反向 API

app-server   = 中央网关（所有 UI 的入口 + core 的管理者）
               协议：类 JSON-RPC
               传输：ws/stdio/unix/remote
               角色：Codex 的"大脑壳"
```

### 一句话

```
exec-server 是"手脚"（执行）
app-server  是"大脑壳"（管理 + 通信）
MCP Server  是"反向门"（让外部进来）
```

> 注：本版本代码中**没有** `codex mcp-server` 子命令。如果要让 Codex 对外暴露 MCP 服务，目前只有 TUI 的 `DynamicToolMcpServer`（HTTP 模式）。exec-server 是 JSON-RPC 服务器，不是 MCP 服务器。
