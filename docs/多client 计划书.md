# 多 Client 方向书

状态：方向已定，尚未实施。当前能力见 [STATUS](../STATUS.md)，现有协议与运行约束见 [设计书](设计书.md)。

## 目标

近期完成 Web、Wails Desktop、headless CLI 三端，共用后台行为。**JSON-RPC 2.0 消息格式不变**，包括现有的 `jsonrpc: "2.0"`、请求 ID、方法、参数、结果、错误和通知；只替换消息的传输方式。不复制业务方法或 React 页面。TUI 留待以后复用进程内接入。

```text
Web      React → WebSocket → appserver/WS 接入 ──────┐
Wails v3 React → Stream → desktop/适配 ──────────────┼→ appserver/连接入口 → clientconn → 方法表 → 领域服务
headless Go Client ────────→ 进程内通道 ─────────────┘
                       全程传完整的 JSON-RPC 2.0 消息
```

## 边界

- Client 与 Go 两侧都按传输方式接入。传输适配只负责连接、收发**完整 JSON-RPC 消息**和关闭；不认识会话、工具或具体方法。不为对称而建新框架。
- 共用层负责初始化、请求与响应配对、错误、订阅、通知顺序和断线清理。每个 Client 有独立连接状态；业务事实仍归原有服务。断线不停止已接受的 Run，有副作用的请求不自动重发。
- Go 侧已有 WebSocket 的 `jsonrpc2.ObjectStream` 适配，并接入 `clientconn`。开放同一个连接入口：Desktop 把 Wails `StreamConn` 适配成 `ObjectStream`，headless 使用进程内通道；`Server.Call` 不能代替带订阅与通知的完整连接。
- React 保留一份类型化 RPC Client、手写契约和状态投影；从 `RPCClient` 抽出最薄的消息传输接口，分别接 WebSocket 与 Wails。Wails 原始 Stream 接收的是 `ArrayBuffer`，在适配层解码为 JSON 文本；每帧仍是一条完整 JSON-RPC 消息。headless 有自己的 Go Client，不复制业务方法。
- Desktop 选择 Wails v3：React 在 WebView 中使用 `Stream("rpc")` 收发消息；`clients/desktop` 注册 Go Stream 处理器并适配 `ObjectStream`，不建业务方法绑定。Wails 对普通请求推荐绑定，但这里为共用完整 JSON-RPC 协议而使用 Stream。`appserver` 不依赖 Wails。Stream 打开后发送 `initialize`。沿用现有 16 MiB 消息上限；写入受阻时必须有断线或超时收尾，不能让关闭一直等待。
- Wails 直接加载 `clients/web` 的同一份构建产物，桌面启动不调用 Web 的 `Listen`。Stream 借助 WebView 的资源服务器收发请求，**不是纯进程内函数调用**，但不为桌面业务监听 TCP 端口。普通 Web 仍用 WebSocket。固定并配套验证 Go 模块与前端 runtime 的 v3 Beta 版本。

## 大致步骤

1. 在前端隔开 JSON-RPC 处理与 WebSocket 收发，保持现有 React 调用和 Web 行为。验证初始化、请求超时、通知及断线恢复。
2. 在 Go 侧增加进程内消息通道的接入，复用现有 `clientconn`；验证请求、响应先于订阅通知、取消、慢 Client 和关闭清理。不要另造业务分发器。
3. 接入 headless CLI：同一装配与进程内入口，接收运行通知、支持停止、输出结果与退出码；不绕开 appserver 直接调用领域服务。
4. 接入 Wails v3：共用 React 页面，加入 Stream 传输适配、桌面 Go 接入、窗口启动与关闭。先验证 JSON-RPC 往返、通知顺序、页面重载、慢 Client、关闭和消息大小；再检查 Monaco Worker、图片压缩、剪贴板、外部链接与目录选择在实际 WebView 中的表现；确有差异才加平台小适配。需要让独立进程控制已运行后台时再设计本机传输，不提前实现远程服务或 TUI。

## 目录方向

```text
internal/backend/            Web 与 Desktop 的显式装配；不复制服务清单
cmd/harness*/                Web 与 Desktop 各自的启动入口
internal/appserver/          方法表、clientconn、WebSocket／通用进程内接入
clients/contracts/          唯一的手写 TS 业务契约
clients/web/                唯一的 React 页面与构建产物
  src/client/                共用 RPC Client + WebSocket／Wails 传输适配
clients/desktop/            Wails Stream 适配、窗口与打包配置，不复制 React 页面
clients/headless/           Go Client、输出与退出控制，不复制后台服务
```

目录名和文件拆分在施工时按实际依赖确定；不为减少字段或追求对称再建 Host、Product、万能 Client 层。桌面使用通用消息 Stream，不改变手写业务契约。

## 验收边界

- Web 原有功能不变；Web、Wails、headless 收发相同格式的 JSON-RPC 2.0 消息。
- Headless 能独立完成一轮并得到结果／退出码；Wails 能在不开放业务端口的情况下运行同一后台。
- 同一进程中多个 Client 连接时，请求结果只回发起者，订阅事件各自收到；重连以 Snapshot 补齐，不靠传输重放写请求。Web、Desktop、headless 分别启动时不默认共享同一个后台进程。
- 关闭一个 Client 清理其订阅与消息通道，不停止另一个 Client 或已接受的 Run；停止整个应用按现有顺序收尾。

参考：[Wails v3 Streams](https://v3.wails.io/guides/streams/)、[Wails v3 当前状态](https://v3.wails.io/status/)、[Codex 多端通信调查](codex-docs/client-communication.md)。Codex 的进程内通道可作结构参考；Harness 仍保留完整 JSON-RPC 2.0 消息格式。
