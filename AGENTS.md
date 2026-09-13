# AGENTS.md

压缩会话、换人或新开对话时，先读本篇，再按任务读取：

- `STATUS.md`：已经完成的事实与当前可运行状态。
- `docs/设计书.md`：目标产品形状与稳定架构决策。
- `DATA_MODEL.md`：数据归属、持久化与运行时状态。
- `WEB_UI.md`：React Client 的界面、状态与通信边界。

`docs/codex-docs/` 与 `docs/reference/` 只是外部项目对照，不是 Harness 规范。优先读本项目文档，不先翻 DSH / pi 源码。

文档纪律：完成事实只写 `STATUS.md`；稳定结论写设计书；`docs/plan/` 只保留仍在执行的方向和计划，完成后删除或收口。不要维护第二份功能清单。

---

## 1. 当前事实与架构

正式调用链只有一条：

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
- app-server 负责接入、协议、校验、分发、订阅、反向请求和连接清理；不拥有产品业务。
- Product 负责业务编排；不拥有协议、页面组件或公共内核数据。
- kernel 提供执行和数据能力；不依赖 appserver、products、clients、surface 或 plugins。
- Client 只保存界面状态和服务端投影，不成为业务事实来源。

---

## 2. 心智模型

**进程是一张服务表。聊天是宿主中的一个产品，不是根。**

```text
Host（服务表）
├─ 公共内核：session / runner / agents / llm / tools / loops / skills / ...
└─ 后台产品：harnessProduct；以后可以有其他 Product

appserver.Server（Host 外，由入口创建、接入 Product 并管理关闭）
```

服务有两种形态：

```text
整份服务  RegisterService("llm", client)      Resolve 后直接使用
登记处    RegisterService("tools", registry)  其他插件再 Register 条目
```

表上的值一律叫服务。登记处中的一项叫插槽条目。运行后才存在的 Session、Run 等叫活对象。

内核扩展由定义者写契约，提供者和消费者都 import 定义者，彼此不 import。外围 appserver 直接依赖具体 Product 和公共服务。Host 运行时只存服务；类型安全来自 Go 包里的契约。

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

- Go 数据类型与 TS 数据类型分别手工维护；修改接口时同步方法名、字段、可选性和返回类型。Go 端负责实际运行时校验。
- 业务数据放定义者的 `types.go`；网络参数与结果放 `appserver/harness_types.go`，登记、请求处理与错误映射放 `appserver/harness.go`，运行订阅处理放 `appserver/harness_run.go`。共享业务结果直接复用，不复制。
- `appserver.Register` 直接绑定方法名和类型化处理函数，组装时编译输入输出 Schema；不要为只有 Name 的方法再造声明结构体或工厂函数。
- 重名、空处理函数或坏契约使组装失败。
- 入口在所有插件安装、方法登记成功后才启动监听；组装失败则关闭清理，不开放网络接入。
- 手写 TS 契约放 `clients/contracts/`，由各 Client 共用，不自动从 Go 生成。
- Client 按手写契约调用，不提供运行时接口目录；Schema 只用于服务端校验。
- `npm --prefix clients run contracts:check` 检查手写 TS 类型及类型测试，不保证 Go / TS 自动一致；接口改动需对照两端审查。
- 运行时仍校验输入和输出；TypeScript 不能表达的格式、长度等约束以 Schema 为准。
- appserver 直接 import Product 和所需公共服务；Product、kernel 和业务插件不得 import appserver。入口从 Host 取出依赖，调用 `server.BindHarness(product, events)`；appserver 不登记进 Host，不读取 Host，也不依赖具体 Client。
- 不自动暴露 Host 方法；只有显式登记的对外方法可调用。
- `sourcegraph/jsonrpc2` 负责 JSON-RPC 封套、请求 ID、响应与通知；appserver 只保留类型化方法登记、校验和产品调用，不在库外重写一套协议兼容层。
- appserver 只有一个入口 `Server`，方法表、WebSocket 监听和关闭生命周期不再拆成两个 Server。每个 Client 对应一个 `Connection`；它是 IM 网关式的基础设施对象，只保存初始化、RPC 连接、订阅、发送保护和断线清理等瞬时连接状态，绝不保存 Session、Run、设置或任何产品业务状态，也不决定 Start / Steer / Stop 等业务行为。
- `Connection` 断开只清理该 Client 的监听和网络资源，不停止已接受的 Run。需要业务判断的代码一律放 Product 或对应公共服务，不能为了调用方便塞进连接对象。
- 连接先完成协议版本初始化；只监听回环地址并校验浏览器 Origin。本机模式不增加临时鉴权。
- 订阅先接入事件，再取 Snapshot；响应先于缓冲通知发送。重叠账本按 Entry.ID 去重，慢连接断开，不能阻塞 Runner。

线上的单请求、响应与通知使用 JSON-RPC 2.0 字段 `jsonrpc`、`id`、`method`、`params`、`result`、`error`；当前正式 Client 不发送 Batch。请求 ID 只做本次响应配对，不充当业务防重 ID。畸形封套和未支持的 Batch 采用协议库行为，不为无真实消费者的边角输入增加兼容代码。

---

## 4. Product 与公共服务

`products/harness` 是 Harness 后台产品，拥有创建会话、发送、Steer、停止、分叉、快照和产品命令等业务编排。原 `kernel/chat` 已迁移，不保留 ChatService 兼容层。

模型、Agent 设置、Skill、命令目录和事件等公共能力由 appserver 直接调用对应服务，不经 HarnessProduct 做无意义转发。

新增产品时：

- 业务放自己的 Product。
- 产品状态由产品自己的 Store 管理。
- 共用 Runner、Session、LLM、Tools 等公共服务。
- 对外方法在 appserver 内登记，具名处理方法直接调用 Product。
- 接入新产品时在 appserver 增加对应接口文件；不建立产品自己的小 Host，不要求通用插件自动接入。
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
- Loop 每个允许插话的检查点都要取尽当时的外部输入；一次模型输出及其全部工具结果是不可插入的步骤，Steer 与协作回报只能在步骤后的检查点按到达顺序落账。做不到就不能登记该 Kind。
- 用户停止用 Context 取消；停止后不执行尚未开始的工具。
- Stop 是独立控制信号，不进入待提交输入或对话账本；停止时尚未落账的 Steer 被拒绝，协作通知保留在来源服务等待以后重试。
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

运行事件、JSON-RPC 请求 ID、连接和订阅都不是账本事实。后台重启后，未完成运行标记中断，不自动续跑；Client 重新初始化，再通过订阅接口一起取得 Snapshot 和后续事件。

---

## 7. 包与依赖

目标结构：

```text
cmd/harness/          进程组装、启动与关闭
clients/contracts/    手写 TypeScript 契约
clients/test/         无界面的网络验收 Client，不是正式 SDK
appserver/            方法契约、登记、校验、协议与连接
products/harness/     Harness 后台业务
kernel/               公共执行、数据与登记处
plugins/kernel/       内核服务提供者、登记处填充者
clients/web/          React + TypeScript + Vite 与嵌入静态资源
```

依赖方向：

```text
cmd       → appserver / products / kernel / plugins / clients 的静态资源
products  → kernel
plugins   → 自己填充的定义者
appserver → products / kernel 公共服务
appserver 不得 import plugins / Client；不得读取 Host
kernel    不得 import appserver / products / plugins / Client
Client    只依赖手写 TS 契约和自身 UI；不读取 Go Host
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
- 固定启动顺序只写在 `cmd/harness`；用户配置只提供模型、MCP 等运行数据，不重排插件。
- 启动中途失败，已 Start 的插件倒序关闭。
- 启动时固定登记的条目随 Host 一起消失，不需要 unregister；运行期会离场的订阅才返回幂等取消函数。
- Host 只关闭通过 Install 安装的插件；入口直接创建的 app-server 由入口关闭。
- 正常退出先关闭 appserver Server，拒绝新调用、断开连接、解除订阅并等待处理退出，再关闭 Host。已接受的 Run 不继承连接 Context，由 Runner 停止和关闭。

---

## 9. 实现与验证

- 修改前区分：已确认事实、合理推测、未知信息。
- 发现冗余、设计拿不准或准备新增抽象时，先调研 Codex 的对应实现：相关文档 → CodeGraph 定位 → `reference/codex` 关键源码核实。优先复用已验证且适合 Harness 的做法，不凭空再造；索引不可用则直接读源码。借鉴时只取当前需要的部分，不照搬 Codex 的额外能力或复杂度；没有对应实现或约束不同就明确说明，不能把参考当作免验证的答案。
- Bug 尽量先稳定复现或写最小失败测试，再修根因。
- 可读性 > 炫技；简单设计 > 通用设计；当前需求 > 假想未来；最小必要改动 > 无关重构。
- 标识符英文，注释中文，commit 双语。
- 普通包返回错误，不打日志；错误只在进程边界或无法返回的后台入口打印一次。
- `err := f()` 与 `if err != nil` 分行。
- 检查到错误或不满足条件时就地返回，让正常流程按执行顺序向下展开；循环中可用 continue 跳过不适用项。避免先保存多个判断结果、隔几段再处理，以及不必要的 else 和嵌套。以读起来顺畅为准，不机械改写每个分支；调整时保留错误优先级、锁的范围和必要的收尾。
- 优先完整展开、分段阅读：连续流程就地按执行顺序书写，用空行与简短中文组注释区分阶段，允许函数稍长，不以主函数短作为可读性目标。只有消除实质重复或分离完整职责时才提取方法，不为判断、加锁或组装步骤建立 helper 链。锁的作用范围、错误返回与 defer 收尾应直接可见；检查状态与修改状态保持原有原子性。
- 同一角色保持命名一致，关键对象使用能认出职责的名字；展开挤成一行的方法，避免难区分的缩写。注释解释阶段、原因和边界，不逐行翻译代码或堆砌术语。
- 业务错误在产品层明确分类，对外错误翻译集中在接口边界；避免为映射错误重复执行业务校验。协程的启动、取消和等待应能在同一处看清，读写方法专注各自工作；不以吞错、隐藏状态或省略等待来减少视觉噪音。
- 每包一个主要公开构造入口；构造函数只校验依赖和组装。
- 接口处理优先使用结构体保存依赖、具名方法承载行为，不用捕获依赖的闭包或匿名 Handler；登记名单留在 appserver 的对应产品接口文件，main 只传入依赖并启动。
- Go 的关键能力结构体是理解代码的重要入口。字段应按适合该结构体的顺序排列，并按职责用空行和中文组注释分隔，优先让人一眼看懂它的组成；不强套统一的排序模板。
- 泛型只用于必要的公共类型转换，类型参数写成 `Input / Output`。一次调用的校验、解码、业务调用、编码与输出校验顺序写在同一方法，不拆成绕行的小助手。
- 搜索优先 `rg` / `rg --files`。
- 改代码后执行与风险匹配的单测、race、vet、两端契约审查与前端检查；不要为了通过检查改无关代码。
- 日常广覆盖回归优先使用 `make agent-check`，它按需安装依赖、只构建一次页面并并行执行常用检查；开发中仍先跑与改动直接相关的最小测试，发布前再用 `make test` 做完整串行验收。
- 构建与启动命令以 `STATUS.md` 为准；不把尚未完成写成事实。



## 用户偏好
用户是重度ADHD患者
