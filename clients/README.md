# clients

**一份 React UI，两种启动方式。** Web 在浏览器中显示页面，Desktop 在 Wails 的 WebView 中显示页面；各自的 Go 入口都启动同一套后台。

## 1. 构建：页面如何进入程序

以下路径相对 `clients/`：

```text
main.tsx + ui/ + web/platform.ts + desktop/platform.ts
                         |
                    Vite 构建
                         v
                       dist/
                         |
                 assets.go（Go embed）
                         |
              +----------+----------+
              v                     v
       harness 可执行文件    EDITH 可执行文件
```

两端采用同一套前端构建配置，各自编译时嵌入页面。正式运行不需要 Node.js 或 Vite。

## 2. 启动：谁先启动，谁加载页面

下面是正式版的正常启动流程。Web 直接打开后台；Desktop 先创建 Wails 应用以拦截第二次启动，再打开后台。

```text
Web：启动 harness
  |
  +-> cmd/harness/main.go（Go 进程入口）
       1. backend.Open -> appserver + 领域服务
       2. clients/web 启动 HTTP：127.0.0.1:8888
          /     提供嵌入的页面
          /rpc  接受 WebSocket
       3. 打开系统浏览器 -> 浏览器加载页面 -> 前端启动（见下方）

Desktop：启动 EDITH
  |
  +-> cmd/harness-desktop/main.go（Go 进程入口）
       1. 创建 Wails 应用并登记单实例唤醒
       2. backend.Open -> 数据目录锁 + appserver + 领域服务
       3. 挂载页面与 rpc Stream，创建窗口和托盘
       4. app.Run -> WebView 加载页面 -> 前端启动（见下方）

前端启动（两端共用，运行在浏览器 / WebView 中）
  main.tsx
    -> 选择 web/platform.ts 或 desktop/platform.ts
    -> 注入 openSocket、openExternal、通知能力；Desktop 另接关窗状态桥
    -> 挂载共用 ui/src/App
    -> ChatConnection 建立连接、初始化协议、读取页面数据
```

Desktop 自己启动 Go 后台，业务通信不需要启动 Web 的 8888 监听器。两端默认使用同一个 `~/.harness`，因此不能同时运行两个后台占用它。

### 开发模式有什么不同

| 命令 | 页面从哪里来 | 后台与热更新 |
| --- | --- | --- |
| `make run` | 浏览器访问 Vite `127.0.0.1:5173` | 并行启动 Vite 与 `cmd/harness --no-browser`；同源 `/rpc` 代理到 8888。前端热更新，Go 改动后手动重启。 |
| `make desktop-run` | Wails WebView 加载 Vite 开发页面 | `wails3 dev` 启动 Vite 和 Desktop；前端热更新，Go 改动后重建并重启 Desktop。业务仍走 Wails Stream。 |

开发命令会先准备依赖和嵌入所需的 `dist`；浏览器／WebView 实际显示 Vite 的最新页面。

## 3. 运行：一次操作如何到达后台

```text
浏览器 / WebView：前端 TypeScript
  共用 App -> client/chat.ts、client/rpc.ts
                         |
              发送完整 JSON-RPC 消息
                         |
         +---------------+---------------+
         | Web                           | Desktop
         v                               v
  web/platform.ts                desktop/platform.ts
  WebSocket /rpc                 Wails Stream("rpc")
         |                               |
---------|---------- 前端 / Go 边界 ------|----------------
         v                               v
  web/server.go                  cmd/harness-desktop 的
    -> web/websocket.go          HandleStream 回调
                                   -> desktop/wails.go
         |                               |
         +---------------+---------------+
                         v
            appserver/connection.go：ServeStream
                         |
            初始化、请求分发、订阅与断线清理
                         |
              conversations / 各领域服务
                         |
           需要执行 Agent 时 -> Runner -> Loop / Tools

  响应与订阅通知沿原连接返回 -> 共用状态投影 -> React 更新页面
```

**`platform.ts` 在前端执行，`websocket.go` 和 `wails.go` 在 Go 后台执行。** 后两者只转换传输消息，业务方法由 appserver 统一分发。

打开 OAuth 等外部网址是独立的平台能力：`openExternal(url)` 在 Web 打开新标签，在 Desktop 调用 Wails 打开系统浏览器，不经过上图的业务 RPC 分发。

后台通知也按平台接入：Web 用浏览器 Notification API 和只读 RPC 订阅；Desktop Go 直接订阅进程事件，通过 Wails 原生通知服务发送。开关属于各端本机偏好，不进入会话或 Run。

## 4. 状态与退出

- UI 保存草稿、当前选择和服务端投影；会话、Run 与持久化数据归后台。平台适配器不保存会话状态。
- 浏览器页面关闭或连接断开：清理该连接的订阅、用户终端等资源，已接受的 Agent Run 继续执行。
- Web 后台退出：先关闭 HTTP／WebSocket 并等待 RPC 收尾，再关闭 appserver 和领域服务。
- Desktop 点关闭：Windows／macOS 直接隐藏到托盘，任务、终端与草稿继续保留。Windows 使用自绘标题栏，macOS 保留原生交通灯；窗口操作仍由 Desktop 接入层负责。
- Desktop 显式退出：保护未保存内容；确认后 `app.Run` 返回，后台取消运行并等待资源收尾。退出整个应用与单纯断线不同。

## 源码入口

- [`main.tsx`](main.tsx)：唯一前端入口；[`ui/`](ui/README.md)：共用页面、RPC、订阅和投影。
- [`web/`](web/README.md) / [`desktop/`](desktop/README.md)：两端平台能力与 Go 传输。
- [`cmd/harness`](../cmd/harness/main.go) / [`cmd/harness-desktop`](../cmd/harness-desktop/main.go)：两种 Go 启动入口。
- [`backend`](../internal/backend/open.go) / [`appserver`](../internal/appserver/README.md)：后台组装与公共 RPC 接入。
- [`contracts/`](contracts/README.md)：手写 TS 契约；[`test/`](test/README.md)：无界面网络验收 Client。
- `package.json / package-lock.json / vite.config.ts`：统一依赖与构建；[`assets.go`](assets.go)：两端共用的资源嵌入。

## 外观资源

`ui/src/fonts.ts` 管理本机字体偏好，`ui/src/styles.css` 提供 UI／内容／代码字体 Token。
字体与许可证随构建打包，来源及字宽规则见 [字体说明](ui/src/assets/fonts/README.md)。
`ui/src/workspace/use-session-activity.ts` 订阅后台运行／已读投影；阅读游标由后端保存。
