# AGENTS.md

压缩会话、换人或新开对话时，先读本篇，再按任务读取：

- `STATUS.md`：已经完成的事实与当前可运行状态。
- `docs/设计书.md`：目标产品形状与稳定架构决策。
- `DATA_MODEL.md`：数据归属、持久化与运行时状态。
- `WEB_UI.md`：React Client 的界面、状态与通信边界。
- `docs/plan/app-server-redesign.md`：尚未完成的总体迁移方向。
- `docs/plan/app-server-refactoring.md`：当前实施顺序与验收。

`docs/codex-docs/` 与 `docs/reference/` 只是外部项目对照，不是 Harness 规范。优先读本项目文档，不先翻 DSH / pi 源码。

文档纪律：完成事实只写 `STATUS.md`；稳定结论写设计书；`docs/plan/` 只保留仍在执行的方向和计划，完成后删除或收口。不要维护第二份功能清单。

---

## 1. 当前事实与目标架构

当前仓库处于迁移期：后台第一批已经完成，旧 `surface/web`、`plugins/web`、Templ、HTMX、POST 与 SSE 仍能运行，但只用于过渡。不要继续给旧页面体系增加新架构、新插槽或长期规则。

目标调用链只有一条：

```text
React / Web / Wails / 其他 Client
               ↓ 类型化 Client 方法
       WebSocket + JSON-RPC 2.0
               ↓
           app-server
        ┌──────┴────────┐
        ↓               ↓
HarnessProduct      公共服务接口
        └──────┬────────┘
               ↓ 进程内 Go 调用
        Runner / Session / Agents / 其他内核服务
```

- HTTP 只提供静态资源；业务调用与运行事件统一走 WebSocket / JSON-RPC 2.0。
- Web 使用 React + TypeScript + Vite，不使用 Next.js。Wails 首版复用同一套界面和 WebSocket。
- app-server 负责接入、协议、目录、校验、分发、订阅、反向请求和连接清理；不拥有产品业务。
- Product 负责业务编排；不拥有协议、页面组件或公共内核数据。
- kernel 提供执行和数据能力；不依赖 appserver、products、clients、surface 或 plugins。
- Client 只保存界面状态和服务端投影，不成为业务事实来源。

旧 Web 的真实完成情况写在 `STATUS.md`；目标边界以本篇、设计书和 `WEB_UI.md` 为准。

---

## 2. 心智模型

**进程是一张服务表。聊天是宿主中的一个产品，不是根。**

```text
Host（服务表）
├─ 公共内核：session / runner / agents / llm / tools / loops / skills / ...
├─ 后台产品：harnessProduct；以后可以有其他 Product
└─ appServer：入口直接创建和管理的接入服务
```

服务有两种形态：

```text
整份服务  RegisterService("llm", client)      Resolve 后直接使用
登记处    RegisterService("tools", registry)  其他插件再 Register 条目
```

表上的值一律叫服务。登记处中的一项叫插槽条目。运行后才存在的 Session、Run 等叫活对象。

定义者写契约，提供者和消费者都 import 定义者，彼此不 import。Host 运行时只存服务；类型安全来自 Go 包里的契约。

能独立替换，或确实需要别人填充时，才做接口或登记处。默认使用结构体和直接方法，不为测试或假想未来预建抽象。

---

## 3. App-server 与契约

方法声明不是业务实现，也不是另一层网络。它把“收到某个 JSON-RPC 方法名时，应把哪种参数交给哪个类型化处理函数”明确下来。

```text
Client 调用 create(params)
→ 编码 JSON-RPC 请求
→ app-server 按 method 找到绑定
→ 解码并校验 params
→ 调用进程内 Go handler
→ 校验结果并编码响应
```

- Go 数据类型和类型化方法声明是唯一手写契约。
- 数据放定义者的 `types.go`；方法声明与绑定放 `methods.go`。
- `appserver.Register` 绑定声明和处理函数，组装时编译输入输出 Schema。
- 重名、空处理函数或坏契约使组装失败。
- 所有插件安装完成后 `Freeze`；此前拒绝调用，此后拒绝登记。
- `cmd/contracts` 只读声明，不安装运行服务、不访问用户配置。
- `npm run contracts:generate` 生成目录、JSON Schema 与 TypeScript；生成物不手改。
- `npm run contracts:check` 检查生成一致性与类型。
- 运行时仍校验输入和输出；TypeScript 不能表达的格式、长度等约束以 Schema 为准。
- appserver 不 import products、kernel 提供者、旧 Web 或具体 Client；产品插件负责绑定业务处理函数。
- 不自动暴露 Host 方法；只有显式登记的对外方法可调用。

协议使用标准 JSON-RPC 2.0，保留 `jsonrpc`、`id`、`method`、`params`、`result`、`error`。请求 ID 只做本次响应配对，不充当业务防重 ID。

---

## 4. Product 与公共服务

`products/harness` 是 Harness 后台产品，拥有创建会话、发送、Steer、停止、分叉、快照和产品命令等业务编排。原 `kernel/chat` 已迁移，不保留 ChatService 兼容层。

模型、Agent 设置、Skill、命令目录和事件等公共能力由对应服务直接提供对外处理函数，不经 HarnessProduct 做无意义转发。

新增产品时：

- 业务放自己的 Product。
- 产品状态由产品自己的 Store 管理。
- 共用 Runner、Session、LLM、Tools 等公共服务。
- 对外方法由产品插件登记到 app-server。
- 不修改 app-server 的产品分支，不建立产品自己的小 Host。
- 不把页面组件或 HTML 当作后台插件契约。

狼人杀、多机器人、电影只是检验边界的例子，未明确要求前不实现。

---

## 5. Runner / Loop / Session 铁律

一场对话是一轮 `Runner.Start` / `Run`：

```text
闲时发送   HarnessProduct → Runner.Start → agents.Prepare → Loop.Run
运行中输入 HarnessProduct → Runner.Steer → Loop 在检查点取走
停止       HarnessProduct → Subagents.StopFamily → Runner 取消并收尾
```

- Runner 在 Loop 外：`Runner.Run → Loop.Run →（仅 LLM 类）llm.Stream`。
- Loop 是编译进程序的一种执行程序，不是一场会话。换 Loop = 换 Agent Kind。
- Session 只记对话，可以分叉；todo、审批、游戏状态和界面状态不得写入账本。
- Agent 设置拥有 Kind、SystemPrompt 与普通 Tool；SessionSettings 拥有 AgentID、模型、思考档位和工作区。
- Skills 与 MCP 按当前作用域自动发现，不复制进 Agent。
- `agents.Prepare` 每轮读取实时设置，拼好最终 System Prompt；Runner 只接收成品。
- Client 订阅本轮运行投影，不把账本当实时事件流。耐久事件必须先 Append，再发布。
- 同一 Session 同时只能有一个活 Run；活 Run 只存在 `Runner.live`。
- 不要 `Chat.Followup`、Inbox 或下一轮队列。闲时再 Run，忙时 Steer。

修改 Runner / Loop 时必须守住：

- 每个已发出的 tool call 最终必须恰好配对一个 tool result；取消也要补齐取消结果。
- Loop 每个允许插话的检查点都要取尽当时的 Steers；做不到就不能登记该 Kind。
- 用户停止用 Context 取消；停止后不执行尚未开始的工具。
- 已完成或取消的运行必须释放 live、完成句柄并结束后台工作。
- `Runner.Close` 拒绝新运行、取消现有运行并等待收尾。
- 慢订阅者不能阻塞 Runner。

---

## 6. 数据边界

数据归属以 `DATA_MODEL.md` 为准：

```text
Session          对话事实
SessionSettings  本会话如何运行
Agent 设置       一个 Agent 如何工作
Product/插件     自己的业务状态
Client           临时界面状态与服务端投影
app-server       连接、订阅、请求配对等瞬时状态
```

运行事件、JSON-RPC 请求 ID、连接和订阅都不是账本事实。后台重启后，未完成运行标记中断，不自动续跑；Client 重新取得 Snapshot 后再订阅。

---

## 7. 包与依赖

目标结构：

```text
cmd/harness/          进程组装、配置、启动与关闭
cmd/contracts/        只读契约生成入口
appserver/            方法契约、登记、校验、协议与连接
products/harness/     Harness 后台业务与对外方法绑定
kernel/               公共执行、数据与登记处
plugins/kernel/       内核服务提供者、登记处填充者
clients/web/          React + TypeScript + Vite（迁移时建立）
```

当前的 `surface/web` 与 `plugins/web` 是待迁移旧实现，不是目标目录模板。

依赖方向：

```text
cmd       → appserver / products / kernel / plugins / clients 的静态资源
products  → appserver / kernel
plugins   → 自己填充的定义者
appserver 不得 import products / kernel / plugins / Client
kernel    不得 import appserver / products / plugins / Client
Client    只依赖生成契约和自身 UI；不读取 Go Host
定义者    不得 import 填充者
```

一个领域默认同包分文件，不先拆子包。只有“可独立替换、边界稳定、不会绕圈转发”三项都成立，才增加一层目录。不要创建 `manager.go`、`utils.go`、`common.go`。

类型分类：

```text
数据      别人要造、要读的形状       → 定义者 types.go
契约      别人要实现或填充的口       → 定义者 types.go
活对象    挂在 Host 上或运行中的对象 → 与其方法同文件
包内私货  只有本包使用               → 谁使用放谁旁边
```

每个导出类型的注释首句标明“数据。/ 契约。/ 活对象。”。接口方法按职责分组，用中文组注释与空行隔开。

---

## 8. 插件、启动与关闭

插件只负责组装：`Name + Start + Close`。`plugin.go` 不放业务类型和业务实现。

- Start 解析依赖、构造服务、登记服务或条目。
- 谁打开长期资源、启动 goroutine 或监听器，谁负责 Close、取消并等待退出。
- 固定启动顺序只写在 `cmd/harness`；yaml 只选择已编译提供者或可选插件，不重排。
- 启动中途失败，已 Start 的插件倒序关闭。
- 启动时固定登记的条目随 Host 一起消失，不需要 unregister；运行期会离场的订阅才返回幂等取消函数。
- Host 只关闭通过 Install 安装的插件；入口直接创建的 app-server 由入口关闭。
- 正常退出先让 app-server 拒绝新调用并等待在途请求，再关闭 Host；连接清理按接入层实施计划补齐。

---

## 9. 实现与验证

- 修改前区分：已确认事实、合理推测、未知信息。
- Bug 尽量先稳定复现或写最小失败测试，再修根因。
- 可读性 > 炫技；简单设计 > 通用设计；当前需求 > 假想未来；最小必要改动 > 无关重构。
- 标识符英文，注释中文，commit 双语。
- 普通包返回错误，不打日志；错误只在进程边界或无法返回的后台入口打印一次。
- `err := f()` 与 `if err != nil` 分行。
- 每包一个主要公开构造入口；构造函数只校验依赖和组装。
- 搜索优先 `rg` / `rg --files`。
- 改代码后执行与风险匹配的单测、race、vet、生成一致性与前端检查；不要为了通过检查改无关代码。
- 旧 Web 迁移期间的具体构建命令以 `STATUS.md` 为准；新 Client 建立后再替换，不把尚未完成写成事实。
