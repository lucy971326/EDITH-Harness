# App-server 与多 Client 方向

更新于 2026-09-08。方向已确定；完成事实只看 `STATUS.md`，剩余施工从实施计划第三步继续。

## 1. 目标

**一个后台、一份业务、一份契约，多个平等 Client。**

```text
React Web / Wails / 其他 Client
              ↓ 类型化 Client 方法
      WebSocket + JSON-RPC 2.0
              ↓
          app-server
       ┌──────┴────────┐
       ↓               ↓
HarnessProduct      公共服务接口
       └──────┬────────┘
              ↓ 进程内 Go 调用
     Runner / Session / Agents / ...
```

- 保留 Go 静态插件内核，不做动态加载、插件市场或 AI 热改代码。
- app-server 是统一接入层，不是聊天产品，也不直接暴露 Host 服务表。
- Product 组织自己的业务；公共模型、Agent、Skill 等接口直接接入所属服务。
- 后台只有一个 Host；新增产品不建立小 Host，也不修改 app-server 的产品分支。
- 后台传数据与事件，不传 HTML、Templ 或 React 组件。

## 2. Client

- Web 使用 React + TypeScript + Vite，不用 Next.js。
- 普通 TypeScript 管连接、类型化调用、状态归并、订阅与恢复；React 主要负责显示和局部交互。
- HTTP 只提供 Vite 静态资源；业务与事件统一走 WebSocket。
- Wails 首版复用同一套前端和 WebSocket，不另做 IPC / Streams 业务适配。
- Vite 产物由 Go embed 打进二进制，用户运行时不需要 Node.js。
- Client 只保存界面临时状态与后台投影，不成为业务事实来源。
- 各端平等，可查看、发送、Steer、停止和回答后台请求；关闭界面不停止后台任务。

旧 `surface/web`、`plugins/web`、Templ、HTMX、POST、SSE 和页面 demo 插槽只属于迁移期，不继续扩展；React 迁移完成后整体删除，不维护双轨。

## 3. 一份契约

```text
Go 数据类型 + 类型化方法声明     手写 TS 契约
              ↓                      ↓
       appserver.Register         各 Client 共用
              ↓
       处理函数与 Schema 校验

       接口修改时人工同步两端
```

- Go 与 TS 分别手工维护同一套接口约定；TS 放 `clients/contracts/`，修改时同步两端，不保留自动生成链。
- 只有显式登记的方法对外可用，不自动暴露内部服务方法。
- 登记时绑定类型化 handler 并编译 Schema；入口完成组装后启动监听。
- 运行时验证输入和输出；TS 类型不替代业务校验。
- Client 按手写契约调用已适配的功能；未知可选通知允许忽略。

当前基线见 `STATUS.md`；进程内分发与手写 TS 契约不代表网络协议和完整产品 API 已经完成。

## 4. 协议与连接

- 使用标准 JSON-RPC 2.0，保留 `jsonrpc` 字段。
- 支持 Client 请求/响应、双向通知和服务端反向请求。
- app-server 管初始化、请求配对、订阅、待回答请求、断线与慢连接清理。
- 业务服务决定为什么提问、回答是否合法以及之后如何继续。
- JSON-RPC `id` 只配对一次响应；有副作用操作另用业务操作 ID 防重。
- 首版本机单用户、多 Client，只监听回环地址；不做跨设备、账号和逐接口权限系统。
- 本机连接校验、后台发现和唯一实例的具体方式在实现时用最小方案确定。

## 5. 运行与恢复

- 列表只同步简要状态；任务详情、文字增量和待回答问题按查看需要订阅。
- Snapshot 与事件必须无空档衔接。重连后恢复历史、当前运行和待回答问题，再接续事件。
- 无需重放断线期间每个 Delta，但不能漏耐久事实或重复推进业务。
- 慢 Client 不能阻塞 Runner；断开后重新同步。
- 断线不取消已接受任务；明确 Stop 才取消。
- 后台重启把未完成运行标记中断，不自动续跑；旧连接、订阅和待回答请求失效。
- 同一个待回答问题只接受一次有效回答，停止后旧问题不能作用于新一轮。
- 无人回答默认继续等待，只暂停依赖回答的执行；其他任务继续。

## 6. 产品迁移

HarnessProduct 首版只迁移已有正式业务，不借机加功能：

- 项目与会话：创建、列表、查询、历史、分叉。
- 运行：发送、Steer、停止、当前状态、运行事件。
- 设置：SessionSettings、模型、思考档位、Agent。
- 输入能力：图片、命令、Skill 候选。
- 公共查询：模型、Agent、Skill、命令目录直接接入所属服务。

旧页面插槽和 demo 不是迁移目标。视觉 Token、结论优先的消息投影、工具配对、Markdown 清洗等已经验证的体验迁移到 React。

## 7. 完成标准

- 真实 Client 通过 WebSocket 调用真实 Runner，不只是进程内空接口。
- 两个 Client 操作同一会话时不串任务、不重复执行，停止不误伤新一轮。
- 重连、慢连接、无人回答、后台重启和关闭顺序均有验证。
- React Client 覆盖已有正式功能；Wails 复用同一界面。
- 旧 Web 代码和依赖被删除，仓库只剩一条前端与业务通信链。

实施顺序见 `app-server-refactoring.md`。Codex 仅作对照：[Client 通信与多端协作](../codex-docs/client-communication.md)。
