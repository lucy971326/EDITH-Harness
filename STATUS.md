# 项目状态

更新日期：2026-09-12

## 现在是什么

Harness 的内核和聊天业务已具备运行能力。旧 `surface/web` / `plugins/web` 使用 Templ、HTMX、POST 与 SSE，仍是默认启动入口；3A 移除 stepSeq 后，其实时过程会错位，不代表旧页面仍完整可用，也不是目标前端架构。

App-server 第一、二步已经完成：`products/harness` 接替原 `kernel/chat`，类型化接口接受 Schema 校验，Go / TS 分别手工维护；本机 WebSocket / JSON-RPC 2.0 已接通真实产品与 Runner，具备初始化、会话操作和运行订阅。Web 迁移第 3A 步已让快照恢复账本、生成中草稿和明确运行状态；`clients/web` 已接通项目与会话，聊天页面尚未接入（3B）。当前尚未实现反向请求、完整公共服务 API、业务防重。正式启动仍打开旧 Web。

3B 的交互与最小接口方案已拍板并保存到 [文字聊天接入计划](docs/plan/3B-文字聊天接入.md)，尚未实现；用户指定压缩会话后由 Codex 直接实施本步。

```text
当前：浏览器 POST → 旧 Web → HarnessProduct → Runner → ReAct Loop → LLM / Tools
      浏览器 SSE  ← 旧 Web ← Runner 稳定事件 ← 完整消息先落账
      TS 测试 Client ↔ WebSocket / JSON-RPC 2.0 ↔ app-server ↔ 同一个后台产品与内核

目标：React Client → WebSocket / JSON-RPC 2.0 → app-server
                                             ├→ HarnessProduct
                                             └→ 公共服务 → kernel
```

运行 `go run ./cmd/harness` 会读取 `harness.yaml`，组装完整服务链，并在 `http://127.0.0.1:8888` 打开 Chat。

## 已完成

### React Web 交互原型（非正式 Client）

- 用户已确认本版原型为正式迁移的 UI 基准；统一字号为 12 / 13 / 14 / 18 / 24 的 rem 等效值。确认不代表真实后台功能已接入。
- 迁移文档已收口为产品定义与七步计划；用户选择亲自转交任务与回报，已删除当前任务文件。旧方向与旧施工计划已合并，未确认的桌面、多 Client 与反向请求范围仍保留在新计划中。
- 辅助工作区已独立于聊天页：空态选择文件/浏览器/终端，支持创建、切换、关闭标签和“＋”菜单；收起再打开保留本次页面标签，标签内容仍为未接入提示。
- `prototypes/web` 提供独立 React + Vite 原型，实际使用 shadcn Registry 组件、Stone 亮暗语义 Token、Tailwind、Lucide 和系统字体；不修改原有 Go 服务或接入真实模型。
- 可体验项目/会话、草稿、发送与直接 Steer、停止、三级工作过程展开、正常完成收起、模型与思考合并菜单、图片附件、回答分叉、外观与 Agent 设置，以及空白辅助区的展开和调宽。命令/技能只插入示例文本。
- 示例状态覆盖空会话、运行、完成、失败、停止、断线重连。浏览器实测了草稿保留、Steer/停止、完成折叠、模型图片限制、Agent 保存、附件/分叉/项目创建与宽窄屏辅助区；类型检查与静态构建通过。原型不等于正式 React 迁移完成，目录选择、工具输出和回答均为模拟。

### Web 迁移第 1 步：正式前端空壳

- 已从拍板原型建立独立 npm 工程 `clients/web`：React + TypeScript + Vite，沿用 shadcn Registry 组件、Lucide、系统字体和 Stone 亮暗 Token；五档字号仍为 12 / 13 / 14 / 18 / 24 的 rem 等效值。
- 去掉示例项目、会话、回答、Agent、模型、工具输出，以及模拟运行、停止、重连、伪造用量和“原型演示”入口。未创建 RPC Client，未改 Go 后端、手写契约、测试 Client、旧 Web 或 `prototypes/web`。
- 项目列表为空态；打开项目、发送、Steer、停止、分叉、Agent 增删改均禁用，就近标明“后台尚未接入”。Agent / 模型／思考显示“未加载”，用量为“—”。输入可编辑，Enter 不发送、不清空。外观设置可用。辅助工作区仍是标签壳，文件／浏览器／终端只提示尚未接入。
- 验证：在 `clients/web` 执行 `npm ci` 与 `npm run build`（`tsc -b` + Vite 生产构建）通过。浏览器打开 `http://127.0.0.1:5173`，检查了宽屏、窄屏覆盖层、亮暗主题、设置导航、输入框与草稿保留、本地图片预览、辅助区创建／切换／关闭及收起后保留标签；未接入按钮没有造出会话或发出业务请求。字号实测为 12 / 13 / 14 / 24 px。本步未接 WebSocket，也未改根目录构建入口。

### Web 迁移第 2 步：接通真实项目与会话

- `clients/web` 使用浏览器原生 WebSocket 调用手写契约；先 `initialize({protocolVersion: 1})`，成功后才允许 list / create / get / workspace/select。组件调用具名方法，不在 JSX 中拼 JSON。普通请求 10 秒超时；目录选择不套短超时。断线拒绝未完成请求，不自动重发；提供连接状态和手动重连，不做自动重连或运行订阅。
- 开发页连接同源 `/rpc`，Vite 代理到 `127.0.0.1:8889`。代理先核对原始 Origin 与当前页 Host，通过后才 `changeOrigin` 并改写成后台地址；开发和 preview 共用同一套守卫。后台 Origin 校验保持关闭跨源。生产端点仍由通信入口使用当前页同源 `/rpc`；未改 Go embed 或正式静态入口。
- 新增 `workspace/select`：输入 `{}`，成功 `{canceled:false, workspace}`，取消 `{canceled:true, workspace:""}`，真实失败为 RPC 错误。实现放在 appserver 私有平台文件，不 import 旧 Web/plugin，不进入 Product 或 kernel。旧选择器暂留。目录选择本身不创建会话；前端再调现有 create，空会话复用仍由 Product 决定。
- 左栏按 `settings.workspace` 分组、组内 `createdAt` 倒序、可折叠。打开项目：选目录 → create → 刷新列表 → 选中新会话。项目旁「＋」对该 workspace create。首次加载不自动选中。点击会话 get，更新标题和 Agent／模型投影；快速切换时旧 get 不能覆盖新选择。选中后只提示「聊天内容将在下一步接入」，不把已有历史说成空会话。发送、Steer、停止、模型／Agent 修改仍禁用。
- 草稿和图片预览按会话存在前端内存；未选中时的临时草稿单独保存。切换聊天／设置不丢草稿。重连后会话仍在则重新 get；已不存在则清除选择并提示。查询中断线或超时保留当前会话和草稿，只在明确「会话不存在」时取消选中。
- 异常 JSON-RPC 响应先校验再结束等待；目录选择无短超时时，格式错误也会拒绝 Promise，不会一直占着按钮。
- Windows 目录弹窗的取消改用所属 STA 线程的消息定时器调用 `IFileDialog.Close`；移除跨线程 watcher，Show 返回后撤销定时器状态再释放对象。增加已取消 Context 与交互式 Windows 原生取消测试入口；未在 Windows 实机运行原生弹窗测试。
- 上述修复后，相关 Go 测试／race／vet、契约与 RPC 验收、前端 16 项测试和生产构建重新通过；Windows amd64／arm64 的 appserver 测试程序交叉编译通过，不等于原生测试已运行。Windows 验收命令见 `clients/web/README.md`。
- 验证：`go test ./appserver ./products/harness`、`go test ./appserver -race`、`go vet ./appserver ./products/harness` 通过。`npm run contracts:check`、`npm run rpc:check`、`npm run rpc:test` 通过。`clients/web` 的 `npm test` 与 `npm run build` 通过。新增回归：代理拒绝 `https://attacker.example` 且不改写 Origin；`shouldClearSessionOnGetError` 只对 not-found 清选择；畸形 error 结束无超时的 `workspace/select`；目录选择 Context 取消返回错误。隔离 `HOME=/tmp/harness-step2-home` 启动后台，不使用 `~/.harness`，不调用外部模型。浏览器经 Vite 代理实测：已连接后列出按工作区分组的真实会话且首次不选中；直接连接 `ws://127.0.0.1:8889/rpc` 被 Origin 拒绝；切换、设置往返、图片附件不串草稿；项目「＋」复用空会话；断线禁用后台操作并保留草稿；手动重连恢复列表与所选会话；所选会话删除后重连会清除选择并提示「所选会话已不存在」，未选中草稿仍在。原生文件夹窗口未做自动化点击，只覆盖了替身选择器的成功／取消／失败与 Context 取消 Go 测试。本轮四项修正未重做完整浏览器验收。

### Web 迁移第 3A 步：后台聊天恢复能力

- 生成开始时分配 `Entry.ID`；`message-started`、文字／思考增量、内存草稿和最终落账使用同一 ID。定位为 SessionID → RunID → EntryID → BlockSeq。`Entry.Seq` 仍只在落账时分配。已删除用 `stepSeq` 对应落账消息的字段与 Runner 工具位置映射；工具开始／完成用助手 EntryID + BlockSeq 定位原调用，工具结果消息另有自己的 Entry.ID，配对仍靠 `ToolCall.ID`。协作 `MessageID` 语义未改。
- 草稿存在 `liveRun` 内存，快照返回副本。完整消息成功落账后才移除同 ID 草稿；落账与快照用 `handoff` 互斥，不跨 `events.Publish` 持锁。迟到增量若该 ID 已落账则忽略，不重新建草稿。
- Snapshot 含 `entries`、各轮 `runs`（running／success／cancelled／failed／interrupted）、草稿、`updateSeq` 与进程 `seqEpoch`。Client 采用快照后只应用更大序号；耐久消息仍按 Entry.ID 去重。序号按会话、按本进程生命周期保留，结束后不归零；跨重启换 `seqEpoch`。
- 运行结果写入 `sessions/<id>/runs.json`，只保存身份、状态、锚点和错误。写入失败会返回，不宣称已保存。后台重启后未收尾的 running 记录标为 interrupted，不自动续跑。分叉仍为复制节点重新分配 Entry.ID，并按 Seq 重映射运行记录锚点。
- 停止或模型报错保存已产生的正文／思考并标 `incomplete`；不把半截思考改成普通正文，不携带悬空工具调用。`History()` 附「未完成」说明。compact 失败／取消不把半截摘要写入账本。失败发生在完整消息已保存之后不重复追加。
- 未改正式 React 页面、发送按钮或模型菜单。旧 Chat 的 `runview.js` 仍按 `stepSeq` 定位，本步已去掉该字段，旧页面实时过程会错位；第 7 步删除旧 Web。
- 3A 审核修复：快照与运行记录读改写串行，正常结束不会被旧快照改成 interrupted；结束状态与序号原子提交。Steer／协作输入的准入、落账与检查点交接保持互斥，并等待输入通知发布后才释放 Run。已有草稿保留位置，消费插话后的新输出使用新锚点。App-server 运行订阅按连续序号放行并发／重入通知，乱序积压超限断线，Connection 不新增业务状态。
- 修复验证：全量 `go test ./...`、`go vet ./...`、Runner／Session／Persistence／Subagents／ReAct／子任务工具／Product／App-server 相关 race 通过；新增恢复与订阅顺序回归重复 20 次通过。TS 契约与 RPC 检查、真实本机网络验收、Web 的 16 项测试和生产构建通过。回归保留 Steer 发布失败的取消语义，不自动重发、不另起一轮；仍未进入 3B。
- 验证：`go test ./kernel/runner ./kernel/session ./kernel/persist ./products/harness ./plugins/kernel/loops/react ./appserver ./kernel/subagents`、相关 race／vet、`npm run contracts:check`、`npm run rpc:check`、`npm run rpc:test` 通过。隔离数据与本地模拟模型覆盖：半句话快照恢复、中文增量、落账并发无双份、快照边界后不重放已含增量、停止保存半截、未收尾标中断、断线不杀 Run。未在 Windows 实测原生目录弹窗；未做 3B 浏览器聊天验收；未删除用户真实会话。

### App-server 第一批：产品迁移与类型化契约

- 整体迁移 `kernel/chat` 到 `products/harness`，Host 键为 `harnessProduct`；不留旧包或兼容包装。发送、Steer、父子停止、快照、分叉与命令业务仍用原内核执行和存储。
- 模型、Agent 设置、Skill、命令名单与事件订阅的纯转发已移除，调用方直接使用所属公共服务；Agent 设置页测试不再安装 HarnessProduct / Runner。
- 入口在 Host 外创建 appserver，产品插件只安装 Product；入口传入 Product 和事件来源，由 appserver 登记接口，全部组装成功后才启动网络监听。入口关闭 app-server 准入并等待在途调用，再关闭 Host；失败清理不把直接登记的服务误当成插件。
- 已开放 `harness/session/create`、`harness/session/list`、`harness/session/get` 三个进程内接口。保留并发空会话复用与子会话隔离，空列表返回 `[]`；字段、时间与输入输出均通过 Schema 校验，错误有稳定分类。
- Go 类型和方法名提供类型化登记与 Schema 校验；TS 契约在 `clients/contracts/harness.ts` 手工维护，会话接口共用结果与设置类型。`npm run contracts:check` 只检查 TS 类型与类型测试，不再做自动生成或两端一致性检查。
- 第一批验收时，全量 Go 测试、vet、相关包三轮 race 与契约检查通过；保留真实 ReAct / Runner / 工具链的本地模拟模型父子停止回归。该批不调用外部模型，不改 Runner 执行、界面或用户数据；后续网络接入见第二步。

### TS 契约改为手工维护

- 删除 Go → TS 生成入口、生成脚本、生成链专用测试和 `appserver/generated` 的 8 个文件，移除 `json-schema-to-typescript` 依赖。原文件可从 Git 历史恢复。
- `clients/contracts/harness.ts` 集中维护三个会话接口，保留共享结果、字符串时间与空列表参数约束；接口变更需同步 Go / TS。
- 移除仅供导出的 `Describe`、`Definitions`，登记时直接使用私有 `compileMethod`。运行时 Schema 校验、类型化绑定与泛型转换仍保留，不改变业务行为。
- 全量 Go 测试、vet、appserver / Harness 产品 race 与手写 TS 类型检查通过；TS 检查不宣称自动验证两端一致。

### App-server 第二步：最小网络闭环

- appserver 只保留一个 `Server`，同时拥有私有方法表、WebSocket 监听和关闭生命周期；原 `RPCServer + WebSocketServer` 两层及只有 Name 的泛型 Method 工厂已经删除。appserver 在 `harness.go` 用方法名直接绑定自己的具名 handler，再直接调用 Product。
- 使用 `github.com/coder/websocket v1.8.14` 接入网络，`github.com/sourcegraph/jsonrpc2 v0.2.3` 负责封套、请求 ID、响应和通知；删除手写 `protocol.go`、Batch 与畸形封套兼容测试，不在库外重造协议。正式 Client 使用合法的单请求、响应和通知。
- 每个 Client 只有一个基础设施 `Connection`，保存初始化、JSON-RPC 连接、订阅、一个有界事件通知队列和断线清理，不保存业务状态或决定业务流程。协议库持续读取以发现断线，请求仍按连接顺序执行；慢 Client 超限断开，不阻塞 Runner。
- 入口完成插件安装和方法登记后监听 `ws://127.0.0.1:8889/rpc`；本机模式不增加临时鉴权。初始化版本为 1，默认拒绝跨 Origin 握手，10 秒内未完成初始化则断开。正常关闭顺序为 appserver Server → Host。
- 协议内建 `initialize`、`server/unsubscribe`；产品新增 `harness/session/send / snapshot / subscribe / stop`，沿用原有 create / list / get。发送暂时只接文字，闲时 Start、忙时 Steer；已接受 Run 不继承连接取消，不自动重试。
- 移除接口目录 `Catalog / server/catalog`、`Definition`、运行时方法说明与原始 Schema 副本；初始化只返回协议版本，登记表直接保存处理函数。方法说明改为源码注释，产品登记名单合并为一处；解除订阅统一由 Subscription.Close 清理。
- `Product.Send(ctx, RunInput)` 与忙闲协调锁直接归 Product；网络输入输出类型、请求处理、错误映射和事件监听已移到 appserver，删除产品中的 methods.go / handlers.go / run_handlers.go 和 runHandlers 包装。Product 与业务插件不再导入 appserver，Host 不再保存 appServer。旧 Web、手写 TS 契约和测试 Client 的调用形状保持原样。
- 保留断线取消等待中 handler、订阅响应先于通知、极快完成、慢 Client、重复关闭和错误映射回归；断线只清理连接与监听，不停止已接受的 Run。
- 订阅先安装监听再取 Snapshot，响应先入队、随后释放缓冲的 `harness/run/event` 通知。账本重叠按 Entry.ID 去重；解除订阅与断线不停止运行。
- 失败测试复现并修复结束通知已经发布、Snapshot 却短暂把该 Run 显示为活跃的窗口：结束标记在通知前可见，live 仍占用到完整收尾；准备期尚未落账的运行不冒充可恢复快照。
- `clients/test` 是无界面 TypeScript 验收程序，不是正式 Client 或 SDK。Go 集成测试创建临时数据与本地模拟模型，再启动 Node，经过真实 WebSocket → Product → Runner → ReAct 验证创建、查询、发送、极快完成、断线重连继续、忙时 Steer 和停止。停止确实取消模型 HTTP 请求；不调用外部模型或修改真实会话。
- 全量 Go 测试、vet、相关包 race、手写契约检查与测试 Client 类型检查通过。没有实现正式 React UI、反向请求、业务操作防重、完整多 Client 协调，也没有删除旧 Web。

复现网络验收（安装前端依赖后，需 Node.js 22.18+）：

```sh
npm run contracts:check
npm run rpc:check
npm run rpc:test
```

`rpc:test` 自动启动并关闭隔离的本地后台，不需要先启动 `cmd/harness`；普通启动仍保留旧界面，并额外开放上述 RPC 测试入口。

### 迁移期 Web：阶段 1 基础

- `surface/web`：HTTP Server、产品/路由登记处、templ 通用页面壳。
- `plugins/web/chat`：Chat 产品注册及页面。
- 本地嵌入 HTMX `2.0.10`、SSE 扩展 `2.2.4`；Tailwind 编译到 `surface/web/static/site.css`。

### 迁移期 Web：阶段 2 项目与会话

- `SessionMeta` 独立保存 `ID / Title / CreatedAt`；元数据是空会话存在的依据。
- 新会话显示「新对话」；首条用户消息落账后自动改名。
- Chat 按 `Workspace` 分组展示项目与会话；项目不是独立数据。
- Win / macOS / Linux 原生目录选择；取消返回 Chat，真实错误才显示。

### 迁移期 Web：阶段 3 真实聊天

- 启动链已完整组装：`persist → session → llm → machine-local → tools → events → loops/react → skills → skills-builtin → skills-filesystem → agents → commands → runner → subagents → subagent-tools → harness-product → compact → web → chat → chat-composer-skills → chat-composer-commands → panel-demo`。
- 每轮生成 `RunID`，写入本轮耐久消息与 SSE；History Snapshot 和 SSE 共用 RunView reducer / `paint()`。同一 Run 默认合并为一张助手卡，只有耐久 Steer 才切成前后片段；落账完成不会让实时卡片跳位。
- 时间线的耐久顺序使用 `Entry.Seq`；运行中卡片用 `AfterEntrySeq` 定位，Run 内按 `StepSeq / BlockSeq` 排列，工具结果按原始调用块回填。
- Chat 支持普通发送、停止和 Steer；已移除 FollowUp 入口与等待队列，每个活 Run 只执行当前一轮。
- Steer 在接受时立即落账；工具被停止时也会补齐「已取消」结果，不留下悬空工具调用。
- `Runner.Start` 同步占住 Session，并在启动 goroutine 前完成一次 Agent / Skill 准备；准备错误直接返回 HTTP，之后在 Runner 管理的 goroutine 运行；`Runner.Close` 会取消并等待仍在运行的 Run。
- `products/harness` 的 `Product` 是聊天业务入口：创建或复用空会话、下一轮设置校验与启动、Steer/停止、快照、分叉与带会话校验的命令调用由它完成；它不拥有账本或 Run。模型、Agent/Skill 查询和事件订阅由各调用方直接使用所属公共服务。
- Runner 对界面只发布稳定事件：开始、文本/推理 Delta、工具开始/完成、用量、耐久消息、结束状态。
- 每次 SSE 重连重新同步 History；耐久快照会覆盖已排队的旧 Delta，慢客户端被断开，不会阻塞 Run。
- 模型与思考档位是独立选择框；每次普通发送前两者必选，换模型会清空档位并影响下一轮 Run。
- `models.json` 为每条模型手写 `contextWindow` 和 `vision`；`Models()` 带给 Chat，模型下拉用 `data-context-window` / `data-vision` 挂上。当前 DeepSeek 两条窗口 100 万、不看图；Google `gemini-3.5-flash-lite` 窗口 104 万、能看图。
- Chat 在模型选择旁画用量球：每次模型调用结束后，ReAct Loop 读 `ChunkFinish.Usage`，Runner 发 `usage` 事件，走现有 SSE，`chat.js` 更新。已用 = InputTokens + CacheReadTokens。不进账本，刷新后从 0% 开始。
- Chat 可附图（选文件或粘贴）；只发图也行。当前模型 `vision` 决定按钮是否可用。图进账本原样保存；发给不看图的模型时，`llm` 把图换成文字占位。用户气泡由 RunView 画图。当前 DeepSeek 两条都不看图，按钮默认禁用。
- Agent 设置已持久化为 `~/.harness/<agent-id>.agent.json`；`default` 是可编辑、不可删除的新会话默认项，首次生成时显式选中当时全部普通 Tool。Chat 普通发送可切换下一轮 Agent；Steer 不改变正在运行的 Run。Agent 设置页把普通 Tool 清单放在默认折叠的「高级配置」。
- `plugins/web/settings/agents` 填入 Web 公共设置页，使用 HTMX 管理 Agent 的新建、编辑与删除；仍被任一会话选择的 Agent 不可删除。

### 真实 Skills

- `kernel/skills` 现在只登记 Provider；每次按工作区动态 List，稳定合并结果，跨 Provider 同名报错。
- `plugins/kernel/skills/filesystem` 扫描用户 `~/.harness/skills`、`~/.agents/skills` 和项目 `.harness/skills`、`.agents/skills` 的直接子目录；项目覆盖用户，同层 `.harness` 覆盖 `.agents`。
- `SKILL.md` 严格校验 Agent Skills 的 `name`、`description` 与目录名；缺失根为空，坏候选报错，未知 frontmatter 允许。
- 系统、个人与当前项目 Skill 都自动可用，不写入 Agent。Prepare 始终注入摘要与 `SKILL.md` 绝对路径；不以 `read` / `bash` 是否勾选为条件，也不暗中补开这两个工具。
- Skill 正文仍由模型按需使用已启用的普通 Tool 读取，工具调用与结果继续按原路径写入 JSONL。

### 第一版 Chat Skill 输入候选

- `kernel/skills` 增加 `system` 作用域；内置 `skill-creator` 由 Harness 自己提供，启动时物化到 `~/.harness/system-skills/skill-creator/SKILL.md`，所有 Agent 自动可见。
- `agents.Service.AvailableSkills(workspace)` 返回当前作用域全部 Skill；`Prepare` 与 Chat 候选共用同一套可用范围。
- Chat 增加自身的 `composer.suggestions` 登记处；`plugins/web/chat/composer/skills` 把同一 Skill 来源登记给 `/` 与 `$`，内核 Skills 不依赖 Chat。
- 输入 `/` 或 `$` 显示单行 Skill 候选；选择后在 textarea 当前光标处插入 `$skill-name`，发送顺序与文字顺序一致，消息卡在原位置显示图标和名称。
- `kernel/commands` 是平台命令登记处。`compact` 填入后，Chat `/` 列出命令；选中立刻 `Call`，不插入 `/compact`。
- `Runner.Compact` 占用空闲会话，用当前模型生成摘要；只有正常结束且正文非空才追加 `Kind=summary`。`History()` 从最近摘要开始发给模型。聊天区仍画完整账本，压缩卡只作标记。
- 个人 MCP 与项目 MCP 都自动加入本轮工具名单；Agent 不选 Server 或 MCP 小工具。配置按现有加载时机生效，必要时重启。
- 第一版未实现 MCP 输入候选以及 `@`、`!` 输入来源。

### 阶段 4A：右侧面板登记处

- Chat 在 Host 的 `chat` 键提供 `chat.Service`；独立面板插件可登记类型。
- Chat 固定画右侧 Tab 壳、`+`、开关与拖拽调宽；浏览器内存保存当前 Tabs 与宽度，Session 不保存。
- 已安装 `plugins/web/chat/sidepanel/demo`，用于验证外部插件能登记并渲染 `demo:main`；它不冒充文件面板。

### 阶段 4B：消息复制动作

- Chat 在同一 `chat.Service` 提供 `message.actions` 登记处；内置 `copy` 通过它填入。
- `paint()` 统一在耐久用户卡和已完成助手卡底部画复制按钮；运行中的助手卡不允许复制。
- 动作路由返回 `{"text":"..."}`；浏览器把 `text` 写入 Clipboard。
- 助手卡用 `RunID + boundaryEntryID` 在当前分叉账本定位同一 Run 段，遇下一条同 Run 用户消息停止；只复制其中最后一条有正文的助手消息，忽略推理、工具、工具前临时文字和相邻 Steer 段。
- `plugins/web/chat/message/fork` 填入 `fork` 动作：仅在已完成助手回答下出现；它复制截至该回答的耐久历史与 SessionSettings，创建并打开标题为「原标题 · 分叉」的独立新会话，原会话不变。

### 阶段 4C：Dock 持续状态插槽

- Chat 的 `chat.Service` 提供 Dock 登记处；填充插件登记 `ID / Name / Order` 和 `Render(DockContext) templ.Component`。
- Chat 固定在输入框上方画默认折叠的 `<details>` 外壳；Dock 只画内部内容，业务状态不写入 Session。
- `DockChanged{SessionID, DockID}` 通过 `events` 通知 Chat；Chat 复用会话 SSE，以 `dock-{id}` 直接发送 Templ HTML，HTMX `sse-swap` 只替换该 Dock 内容。
- SSE 首次连接和重连会在订阅后重发所有已登记 Dock 的当前 HTML；未知条目不发送，单项渲染失败不影响聊天流。
- 新增测试专用 `plugins/web/chat/dock/demo`：内存计数验证“状态变化 → events → Chat → SSE HTML”的完整边界，正常 `cmd/harness` 不安装它。

### 阶段 4D：输入工具栏插槽 composer.actions

- Chat 的 `chat.Service` 提供 `composer.actions` 登记处；填充插件登记 `ID / Order` 和 `Render(ComposerActionContext) (templ.Component, error)`。
- Chat 固定在输入框表单 `#composer` 底部工具栏左侧渲染插槽容器 `#composer-actions`；单个 Action 渲染失败被隔离并跳过，不影响 Chat 页面渲染。
- 遵循 `templ + 原生 HTML + HTMX` 原则，组件位于 `#composer` 表单内部，禁止嵌套 `<form>`，选项使用 `type="button"`。
- 新增 `plugins/web/chat/composer/demo`：使用原生 `<details>` 下拉菜单提供快捷模版输入，一行原生 JS 将文本填入 `textarea` 并聚焦；已安装至 `cmd/harness`。

### 阶段 4E：Web 公共设置插槽 settings.section

- Web 表面 `web.Service` 提供 `settings.section` 登记处；填充插件登记 `ID / Title / Order` 和 `Render() (templ.Component, error)`。
- Web 左侧边栏最底部固定放置「⚙ 设置」入口；主界面为 Master-Detail 左右双栏结构，左侧列出插件设置项，右侧为主配置区域。
- 左栏切换使用 `hx-target="#settings-content" hx-push-url="true"` 保留 URL 与历史记录；OOB 自动同步左栏高亮项；默认展示 Web 自带的「外观」栏目。
- 错误隔离：单项渲染失败或返回 `nil` 组件优雅展示加载失败提示，绝不影响设置页主体；空状态展示友好指引。
- 新增 `plugins/web/settings/demo`：登记演示配置（昵称），原生表单通过 HTMX 提交并在内存中维护状态；已安装至 `cmd/harness`。

### UI 重构第一步：视觉基础层

- `surface/web/assets` 已建立统一 Token：亮色、暗色、跟随系统、系统字体、固定字号、4px 间距、统一圆角、边框层级、状态色与减少动态效果。
- 已增加 `ui-*` 公共规则，Web 外壳、设置页与设置 demo 开始消费语义样式，不再为这些页面直接挑白色背景或任意圆角。
- 新增静态编译的 `surface/web/ui` Lucide 图标入口与受控图标常量；全局设置入口与设置空态已迁移。
- 新增浏览器本地主题偏好脚本；Web 自带 `/settings/appearance` 外观栏目，主题不进入 Session、账本或插件状态。
- 本步未改 Chat 的消息投影、SSE、Runner、Session 或插件边界；确认截图后再进入全 Web 迁移。

### UI 重构第二步：全 Web 外壳与填充物迁移

- Chat 页面外壳、Composer、Dock、右侧面板与项目导航已统一消费 `ui-*` 公共规则；HTMX、SSE、路由与插槽行为不变。
- `surface/web/ui` 新增面板与模版图标；Chat 面板定义使用受控的 `ui.IconName`，正式 Templ 图标不再使用字符图标。
- Sidepanel、Dock、Composer、Settings 的 demo 填充物已迁移到公共视觉规则；动态面板脚本只更新样式类，不改变交互逻辑。
- Chat 消息卡与消息动作仍保留现有 `chat.js` 投影，留待第三步按工作流层级统一重构。

### UI 重构第三步：Chat 工作流投影

- Chat 消息区改为“结论优先”：最终回答直接显示，推理、进展说明与工具细节收进默认收起的工作过程。
- 工作过程与工具详情使用原生 `<details>` 两级展开；多个 Step 按 `StepSeq / BlockSeq` 完整保留，工具结果按原始调用块回填。
- 同一 Run 的 Steer 继续按用户消息切成前后片段，片段不会互相串位；运行中、完成、失败与停止使用紧凑状态行，不显示计时器。
- 最终回答只取最后一个不含 tool-call 且已落账的助手 Step；运行中的临时正文不会提前冒充最终回答。
- SSE 与 History 仍进入同一个 `apply() → paint()`，仅保留浏览器内临时展开状态；不改 Session、Runner、SSE 事件名或插件契约。
- Chat 投影与样式已迁移到 `ui-message-*`、`ui-workflow-*` 语义规则，亮暗主题继续复用 Web Token。
- 用户消息与已落账的最终回答共用浏览器端 Markdown 渲染出口；History JSON 与 SSE 最终消息保持同一视觉结果。解析使用本地 `marked`，输出再经 `DOMPurify` 清洗；推理与工具内容仍保持纯文本。

### UI 重构第四步：验收与收尾

- 已检查亮色、暗色、跟随系统、窄桌面、常规键盘焦点与减少动态效果；Web 外壳、设置页与 Chat 均消费同一套 Token 和 `ui-*` 规则。
- 窄于 1120px 时，右侧面板及其开关由 CSS 隐藏，优先保留 Chat 主工作区，不增加响应式 JS。
- 已知例外：右侧面板鼠标调宽把手暂不支持键盘操作；为保持前端 JS 最少，当前明确不做。

### 公共 RunView

- `surface/web/runview` 提供 Templ 运行视图和公共浏览器 reducer；它统一处理 HarnessProduct History Snapshot、SSE Delta、Step / Block 排序、Tool 回填、工作流、Markdown 与展开状态。
- Chat 改用 RunView；Chat 私有脚本只保留 Composer、停止、消息动作和右侧面板交互，资源由 `/assets/chat/` 路由提供。
- Chat 保持一条 SSE：`run` JSON 交给 RunView，`dock-*` HTML 继续由 HTMX `sse-swap` 处理。
- 新的 Runner 驱动 Web 产品可以直接使用 `runview.View`，不必重写流式投影 JavaScript；Snapshot 与 SSE HTTP 路由仍归产品自己。

### 子会话委派第 1 步：Runner 运行接口

- `Runner.Start` 返回稳定的 `RunHandle`：可获取 RunID、配置快照、完成信号与最终状态/错误；Chat 对 Web 仍只返回启动错误，不保存句柄。
- 活跃运行可按 SessionID、RunID 查询本轮配置快照；Runner 不缓存已结束运行。工具调用上下文增加 SessionID、RunID、协议 ToolCallID，身份不从工具 Arguments 读取。
- Steer 使用可重复观察的广播信号，只有检查点消费消息后才换代次；ReAct 每次调用工具时获取当前信号，停止仍通过 Context 取消。
- 初始化历史与后续 Steer 分离，避免重复消费；Compact 始终拒绝 Steer。取消/关闭后不重新开放输入，收尾依次释放 live、完成句柄、结束后台工作。
- 开始事件部分投递失败仍尝试结束通知，错误保留在最终结果。已验收全量 Go 测试、相关 race、vet 和 diff 检查。
- 此步完成运行基础；委派服务与平台工具的后续进展见下节。

### 子会话委派第 2 步：服务与独立存储

- 必装 `kernel/subagents` 整份服务，启动顺序为 Runner → Subagents → HarnessProduct；提供 Options / Spawn / Send / List / Wait / Stop / StopFamily。
- 使用父 Run 快照继承设置，独立创建子 Session；关系先保存，随后创建会话与设置、启动 Runner。子会话不进入普通聊天列表或空会话复用。
- `subagents/tasks/<task-id>.json` 保存版本化任务、逐轮 RunID / 状态 / 最终正文位置和通知集合；重启保留历史，未完成轮次标记中断，不自动执行。
- 单任务状态与写盘串行，旧轮完整收尾后才能开新轮；关闭取消服务上下文并等待在途启动和运行回调。持久化失败由调用、查询、等待及关闭明确报告。
- 已通过全量 Go 测试、相关包 race、vet 与 diff 检查，覆盖极快完成、旧轮收尾、启动中关闭、实际写盘失败和恢复。
- 平台工具、父账本通知、等待插话与父子停止的后续进展见下节。

### 子会话委派第 3 步：工具入口

- 新增 `plugins/kernel/tools/subagents`，在 Subagents 之后静态安装，向现有 tools 登记处填入 options / spawn / send / list / wait / stop 六个 `subagent_*` 工具；插件不拥有后台资源，子 Run 仍由 Subagents 与 Runner 管理。
- 身份取可信调用上下文，不允许模型参数指定父会话、RunID 或工作区；服务拒绝越权及二层委派。Agent / 模型 / 档位分别继承父 Run 快照，显式覆盖独立校验。
- 工具沿用普通 Tool 权限，安装不替已有 Agent 勾选；需在 Agent 工具清单手动启用。options 返回现有 Agent 设置与模型清单，不新增“用途”字段或第二份名单。
- send 只允许非空文字，忙时追加、闲时开启下一轮；可能已经接收的错误不会自动重发。list 返回含逐轮最终正文的 TaskView 查询投影，正文仍只存在子账本。
- 本步接入单孩子当前轮的基础等待；后续多任务等待、用户插话和自动通知见第 4 步。
- 已通过全量 Go 测试、相关包 race、vet 和 diff 检查；覆盖六工具调用、权限、参数、身份隔离、继承与覆盖、忙闲追加、保存失败反馈和取消。

### 子会话委派第 4 步：完成通知与同轮等待

- 子任务完成后，先可靠保存逐轮状态、最终正文位置与稳定通知 ID，再通过 Runner.Receive 投递到父账本；失败、取消、中断不冒充成功答案。通知身份与轮次在恢复时校验。
- 账本增加 collaboration 消息来源：稳定 messageID、来源 SessionID / RunID。Runner 按实际账本去重，模型侧带明确来源说明，按普通输入处理。通知确认写盘失败可重试，不重复落账或注入正文。
- 父运行中在检查点接收；最终检查点关闭后保留待投递；父闲置或重启均不自动执行。下一次用户发送启动父会话时，待投递通知在初始 History 中进入首次模型请求。
- subagent_wait 改用 taskIDs 数组与可选 seenNotificationIDs，等待任一孩子当前或后续轮次的新完成结果。默认 60 秒、范围 0～60 秒；返回状态、轮次和通知 ID，正文统一走通知。用户 Steer 提前唤醒，取消 Context 结束等待，超时不停止孩子。
- 通知订阅、状态广播和重试后台由 Subagents 拥有，Close 解除订阅并等待退出；Runner 收尾等待在途协作消息发布，并保留落账或发布错误。未新增服务键、SSE 事件或前端 JS。
- 已通过全量 Go 测试、vet、相关包 race 和 diff 检查；覆盖自动投递、多个完成、最终检查点、重启待投递、确认写盘失败重试、用户插话与取消。真实 ReAct / Runner / 工具链连接本地模拟模型，验证等待期间无父模型请求、恢复后正文仅出现一次。
- Chat 父子停止接线和并发派生协调见第 5 步。

### 子会话委派第 5 步：父子停止与启动关闭

- Chat 用户停止经 Subagents.StopFamily 取消父与孩子；父闲置时仍能停止孩子，父正常回答结束不取消孩子。
- 停止边界属于 Subagents 业务状态：内存中的停止代次、父 RunID 与孩子停止标记拦截旧 spawn / send，不建立协作级 Context。send 的父 RunID 来自可信工具上下文；新父 Run 不会放行旧指令。
- StopFamily 不等待孩子启动写盘锁；子 Run 的开始事件补做准入检查。单独停止孩子保留历史，父之后明确 send 可以开新轮。
- Runner 在准备设置前登记 live，停止和关闭覆盖准备期；StopRun 只取消指定 RunID，避免误停下一轮，进入 Loop 前再次检查取消。
- 等待随父停止退出，通知不注入被停止的 Run，也不自动启动父会话；Subagents 关闭继续拒绝新请求、取消孩子并等待在途入口及后台退出。
- 本地模拟模型连接真实 Chat / ReAct / Runner / 工具链，验证父等待中停止、父闲置后停止、剩余工具未执行，以及重新读盘后调用结果完整；另覆盖阻塞启动、旧 send 跨新父轮次及单孩子停止竞态。
- 全量 Go 测试、vet、diff 检查通过；Runner / Subagents / Chat / 委派工具 / ReAct 的 race 测试连续三轮通过。
- 用户已确认启动试用成功。

### 子会话委派第 6 步：验收与文档收尾

- 用户试用成功并确认首版完成；真实使用结论来自用户反馈，不宣称额外执行过未记录的外部模型场景。
- 自动化验收结果见上述各步；核心机制已归入设计书，数据归属保留在 DATA_MODEL。
- 已删除六步施工计划与早期 Subagent 方案，从后续计划移除已完成入口；首版不增加专用子任务面板。

## 验证

- `go test ./...` 通过。
- `go vet ./...` 通过。
- `git diff --check` 通过。
- `npm run contracts:check` 检查手写 TS 契约与类型测试；不再包含生成一致性或 Go → Schema → TS 映射测试。
- `npm run rpc:check` 检查 TS 测试 Client，`npm run rpc:test` 执行隔离的真实网络链路验收。
- `node --test plugins/web/chat/static/test/sidepanel.test.js` 通过。
- `node --check surface/web/static/runview.js` 与 Chat 私有脚本通过。
- 已做真实浏览器页面与布局检查。

## 下一步

实施顺序与范围见 `docs/plan/Web迁移计划.md`，每步任务与审核通过用户在会话中转交；不在本文件重复维护步骤。UI 基准见 `docs/plan/ProductDefine.md` 和 `WEB_UI.md`，稳定架构见 `docs/设计书.md`。

## 运行前提

- Go 1.25。
- 当前旧 Web：修改 `.templ` 后运行 `go tool templ generate`；修改样式后运行 `npm run web:build`，首次需要 `npm install`。
- React 前端：`cd clients/web && npm ci && npm run dev`，默认 `http://127.0.0.1:5173`，开发代理把同源 `/rpc` 转到 `127.0.0.1:8889`。需同时 `go run ./cmd/harness`。不替代旧界面。生产构建为 `npm run build`。根目录构建入口尚未改为 embed 这套产物。
- 数据根目录固定为 `~/.harness`；全局 `config.yaml` 留在根目录，每场会话位于 `sessions/<session-id>/`，其中分别保存账本、元数据与 SessionSettings。项目内旧 `.harness-data/` 和用户目录旧平铺会话文件均不再读取，可由用户自行删除。
- machine-local 直接操作本机文件和进程，没有沙箱与路径限制。
- 本机需要 `~/.harness/config.yaml` 配置 LLM Provider：

```yaml
providers:
  deepseek:
    apiKey: <your-key>
    # baseURL: <optional>
```

## 近期提交

- `3378c7c feat: add chat sidepanel registry`
- `347b9b3 fix: keep chat run segments stable`
- `c9730b9 feat: add ordered chat timeline`
- `e08d515 docs: document real chat runtime`
