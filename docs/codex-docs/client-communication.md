# Codex：Client 通信与多端协作

核对日期：2026-09-07。依据本地 `reference/codex` 快照，不代表未来版本。本文记录源码事实，不是 Harness 的实施方案；伪代码仅解释机制。

## 1. 一句话

**多个 Client 连接同一个 app-server，共用后台会话；请求结果回发起方，会话变化发订阅方。**

```text
TUI 页面与交互
      ↓
AppServerSession → AppServerClient
                    ├─ InProcess：内存通道 → 内嵌 app-server
                    └─ Remote：WebSocket / Unix Socket → 独立 app-server
                                                         ↓
                                          请求处理器 → 后台会话与运行
```

- TUI 通过统一 Client 方法发送请求、接收事件、回答服务端提问，不在页面各处直接操作 socket。
- 进程内请求可直接传强类型对象，跳过 JSON 解析，但进入相同的请求处理语义。
- TUI 支持内嵌、本机 daemon 和远程连接；本机 daemon 连接失败有回退内嵌的路径，不应笼统说 TUI 永远内嵌。
- Desktop、VSCode、Web 客户端源码未在本次核对范围内，不推断其内部连接方式。

源码：[TUI 连接选择](../../reference/codex/codex-rs/tui/src/lib.rs)、[统一 Client](../../reference/codex/codex-rs/app-server-client/src/lib.rs)、[TUI 会话封装](../../reference/codex/codex-rs/tui/src/app_server_session.rs)。

## 2. 消息如何来回

协议借用 JSON-RPC 的结构，但不发送 `jsonrpc: "2.0"` 字段。

```text
连接：initialize（身份与支持范围）→ 收到响应 → initialized

Client 调用：
    id = 新编号
    pending[id] = 等待结果的回调
    发送 { id, method, params }

Client 接收循环：
    Response / Error → 用 id 取出 pending，交回结果
    Notification     → 更新界面
    ServerRequest    → 处理提问或其他交互，用原 id 回答
```

- 远程连接将 JSON 消息装入 WebSocket 文本帧；Unix Socket 路径也使用 WebSocket 握手与帧，不是裸 JSONL。
- `turn/start` 的响应不是整轮最终答案；后续文字、工具进展和结束通过通知到达。
- Client 对无法识别的服务端请求返回“不支持”错误，不能当通知静默丢弃，否则等待方无法正常收尾。

源码：[远程 Client 收发与初始化](../../reference/codex/codex-rs/app-server-client/src/remote.rs)、[消息结构](../../reference/codex/codex-rs/app-server-protocol/src/rpc.rs)。

## 3. 多端看同一会话

```text
服务端维护：
    ConnectionID → 连接状态、发送队列、初始化选项
    ThreadID     → 订阅它的 ConnectionID 集合

请求结果 → 指定 ConnectionID
会话事件 → 当前订阅连接
全局广播 → 已初始化连接，再按通知选项过滤
```

- 客户端请求用 `(ConnectionID, RequestID)` 标识；两个客户端都用编号 1 不会串响应。
- 每个连接独立保存初始化信息、实验选项、通知屏蔽范围和 RPC 门控。连接隔离不等于用户权限隔离。
- **新会话创建时，主循环会尝试给所有当前已初始化连接附着监听器。** 因此订阅不全是客户端手动选择；这是 Codex 的产品策略，不必照搬。
- 新会话附着是 best-effort；创建事件接收落后时会告警并跳过补同步，不承诺这条路径绝不漏事件。
- 慢连接有独立出站队列；可主动断开的连接在队列满时被断开。不能概括成所有传输都采用相同的非阻塞策略。

源码：[连接与出站路由](../../reference/codex/codex-rs/app-server/src/transport.rs)、[会话订阅表](../../reference/codex/codex-rs/app-server/src/thread_state.rs)、[主循环自动附着](../../reference/codex/codex-rs/app-server/src/lib.rs)。

## 4. 多端操作与回答

**操作：** 同一会话的 start / steer / interrupt 请求进入同一串行队列；处理提交请求，不是等整轮生成结束。Steer 核对预期 TurnID，旧界面不能把插话误投给新一轮。忙时如何处理仍由具体业务方法决定。

**回答：** app-server 保存问题、请求 ID、所属会话和回调，可定向发给多个连接。

```text
A、B 都看到问题 Q
A 回答 → 加锁取出并移除 Q → 回调只交付一次
B 随后回答 → Q 已不存在，不再交付
问题处理后 → 向订阅端发 serverRequest/resolved
TUI 收到 → 移除待办及对应弹窗
```

这里是“先取到回调的响应生效”，不等于“先收到合法业务答案才生效”；错误响应也可能消耗回调，业务校验另行负责。

重新加入会话时，后台先发恢复响应，再补充相关状态、重发未回答请求。待回答表是内存状态，不能据此推断进程重启后也会恢复。

源码：[请求串行队列](../../reference/codex/codex-rs/app-server/src/request_serialization.rs)、[Turn 校验](../../reference/codex/codex-rs/app-server/src/request_processors/turn_processor.rs)、[待回答表与重发](../../reference/codex/codex-rs/app-server/src/outgoing_message.rs)、[恢复与已处理通知](../../reference/codex/codex-rs/app-server/src/request_processors/thread_lifecycle.rs)、[TUI 清理弹窗](../../reference/codex/codex-rs/tui/src/app/app_server_events.rs)。

## 5. 断线不等于关闭后台

- 普通连接断开：清理该连接的请求上下文、订阅及连接所属资源；线程处理器不因此直接终止仍存在的会话。不能推广成所有命令、文件监听等连接资源都保留。
- Stdio 是单客户端模式；stdin 关闭有退出服务的路径。内嵌 app-server 也不能视为独立常驻 daemon。
- 多客户端模式启用信号处理时：首次退出信号进入排空阶段，等待运行中的 assistant turns 结束，期间仍接受请求；可强制的第二次信号触发强制退出。
- 因此“优雅关闭总会等所有任务完成”过于绝对；要区分普通断线、单客户端退出、信号排空和强制退出。

源码：[连接清理](../../reference/codex/codex-rs/app-server/src/message_processor.rs)、[会话连接移除](../../reference/codex/codex-rs/app-server/src/request_processors/thread_processor.rs)、[ShutdownState 与主循环](../../reference/codex/codex-rs/app-server/src/lib.rs)。

## 6. 传输限制与调查校正

| 容易误解的说法 | 核对结论 |
|---|---|
| 四种传输方式 | 枚举是 Stdio、UnixSocket、WebSocket、Off；Off 是不启用本地监听，不是第四种通信协议 |
| Off 仅用于 Remote Control | 不能由 Off 推断远程控制已启用；它是独立配置的接入路径 |
| 有 WebSocket，网页就能直接连接 | 当前 TCP WebSocket 入口拒绝带 Origin 的请求；普通浏览器 WebSocket 会携带 Origin，不能原样作为我们的 Web 接口 |
| 所有 Client 都只收主动订阅的会话 | 还存在新会话自动附着到当前已初始化连接的策略；此前本会话讲解漏掉了这一点 |
| 双任务架构 | 是主处理与出站路由分离的简化图；实际还包含每连接读写、请求调度等任务 |
| 粘贴调查中的 Rust 就是原源码 | 部分为简化伪代码，省略错误处理、状态字段和分支，不能直接作为实现复制 |

源码：[传输枚举](../../reference/codex/codex-rs/app-server-transport/src/transport/mod.rs)、[WebSocket Origin 检查](../../reference/codex/codex-rs/app-server-transport/src/transport/websocket.rs)。

## 7. Harness 可借鉴，尚未全部拍板

统一 Client 连接层、连接级请求编号、会话订阅、单次回答交付、待问题重发、慢连接隔离值得借鉴。多 Client 协作已确定要做；自动订阅范围、回答权限、无人在线与后台重启策略仍需自己决定，不自动采用 Codex 默认行为。
