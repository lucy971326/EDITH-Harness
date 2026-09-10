# App-server 迁移实施计划

更新于 2026-09-10。方向依据：[App-server 与多 Client 方向](app-server-redesign.md)。本篇只记录尚未完成的实施顺序；完成事实见 `STATUS.md`。

## 当前基线

第一、二步的完成事实与验证入口见 `STATUS.md`；先完成下面的第二步收口，再进入第三步。旧 Web 只用于过渡，不作为新 Client 的目录模板。

## 职责

| 部分 | 负责 | 不负责 |
|---|---|---|
| app-server | JSON-RPC、连接、初始化、校验、分发、订阅、反向请求与断线清理 | 不决定聊天业务，不直接暴露 Host |
| HarnessProduct | 会话、发送、Steer、停止、分叉等 Harness 业务编排 | 不管理 WebSocket，不返回界面组件，不重复 Runner |
| 公共服务 | Runner、Session、Agents、模型、Skills、Tools 等各自职责 | 不依赖具体 Client，不强制经过 HarnessProduct |
| 产品插件 | 解析依赖并把方法绑定到 app-server | 不把业务实现堆进 `plugin.go` |
| Client | React 界面、类型化调用、Snapshot/事件投影与重连 | 不成为后台业务事实来源 |

```text
Client
  ↕ WebSocket / JSON-RPC 2.0
app-server
  ├─ Harness 方法 → HarnessProduct → Runner / Session / Subagents
  └─ 公共方法    → Agents / Models / Skills / Commands / ...
```

app-server 到处理函数始终是进程内 Go 调用，不是第二次网络请求。

## 第二步收口：用协议库替换手写 JSON-RPC

这是下一次代码工作，先做完再扩充后台 API：

- 使用 `github.com/sourcegraph/jsonrpc2` 接管封套、请求 ID、响应和通知；继续使用 `github.com/coder/websocket`，不采用库内基于 Gorilla 的 WebSocket 子包。
- 删除手写 `protocol.go` 及 Batch、畸形封套等无正式 Client 消费者的兼容测试；正式 Client 只承诺合法的单请求、响应和通知，协议边角采用库行为。
- 将现有 `RPCServer` 收口为类型化 `MethodRegistry`，继续保留登记时 Schema 编译、运行时输入输出校验和稳定业务错误映射。
- `Connection` 只保存初始化、RPC 连接、订阅、一个有界通知通道和断线清理。它不得保存产品状态或执行业务判断；Start / Steer / Stop 等规则只能由 Product 或对应公共服务决定。
- 保留订阅先监听再取 Snapshot、响应先于通知、慢 Client 不阻塞 Runner，以及断线清理监听但不停止已接受 Run。

完成标准：真实网络验收、断线、重连、订阅顺序、慢连接、关闭和 race 检查通过；appserver 生产代码与状态数量必须净减少，不以新的适配层补回被删除的自制协议。

## 第三步：完整后台 API 与多 Client

完成第二步收口后，复用新的最小网络闭环。

补齐 React 迁移需要的正式接口：

- 在已有会话列表与 Snapshot 上补齐正式历史、分叉与 SessionSettings 接口。
- 模型、思考档位、Agent、Skill、命令目录。
- 图片输入、Steer、运行状态与用量。
- 在已有会话订阅/取消基础上补齐任务简要状态和详情状态。
- 服务端反向请求、回答一次性交付与待回答恢复。

同时完成：

- 业务操作 ID 与防重；JSON-RPC `id` 不承担防重。
- 两个 Client 并发操作同一会话的状态检查。
- 旧操作不能停止、回答或修改新一轮。
- 重连 Snapshot + 事件衔接；无需重放每个 Delta。
- 未知可选通知忽略，需要响应的请求明确拒绝。

完成标准：两个测试 Client 不串任务、不重复推进；断线继续、重连恢复、停止和回答边界均通过。

## 第四步：React Web 迁移

建立 `clients/web`，使用 React + TypeScript + Vite：

1. 类型化 Client、连接、初始化、订阅和恢复。
2. 项目/会话导航与聊天主流程。
3. 运行过程、工具回填、Markdown 与图片。
4. 模型、档位、Agent、SessionSettings。
5. 命令、Skill 候选、停止和分叉。
6. 主题、窄桌面、键盘焦点与错误状态。

普通 TypeScript 管通信和投影，React 主要负责显示。迁移保留已有体验，不照搬旧 Templ 组件结构，也不迁移页面 demo 插槽。

完成标准：React Client 覆盖当前正式功能，刷新和重连画面一致，所有业务只走类型化 Client。

## 第五步：桌面与唯一后台

- Go embed Vite 产物，HTTP 只提供静态资源。
- 本地启动器发现或启动唯一后台，不重复启动。
- Wails 承载同一 React UI，首版仍连接 WebSocket。
- 窗口退出不停止任务；提供明确的关闭后台操作。
- 后台启动、发现、本机连接校验与退出顺序形成可重复测试。

完成标准：Web 与 Wails 连接同一后台，任一界面退出不影响已接受任务。

## 第六步：删除旧体系

在 React 功能验收完成后一次删除：

- `surface/web` 与 `plugins/web`。
- Templ、HTMX、旧 POST / SSE 路由与 RunView JS。
- 旧页面插槽、演示插件及仅为它们存在的依赖和生成命令。

不提前删除仍被当前可运行版本使用的代码，也不维护长期双轨。

最后执行：

- 全量 Go test / vet / 相关 race。
- Go / TS 两端契约对照审查与 TypeScript 检查。
- React 构建和前端测试。
- 两 Client、重连、慢连接、停止、反向请求、后台重启与关闭验收。
- 更新 `STATUS.md`、设计书、`DATA_MODEL.md` 与 `WEB_UI.md`，完成后删除本施工计划。

## 不在本计划

- 狼人杀、多机器人或其他新产品。
- 动态插件、插件市场和热加载。
- 跨设备接入、多人账号与复杂权限系统。
- 为假想未来预建第二种传输。
- 新 UI 插槽或迁移旧 demo。
