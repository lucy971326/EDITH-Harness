# 项目状态

更新日期：2026-09-24

## 当前形状

Harness 已完成 React Web 迁移，正式入口只有一条：

```text
http://127.0.0.1:8888/    React 静态页面
ws://127.0.0.1:8888/rpc   WebSocket + JSON-RPC 2.0

React Client
     ↓
appserver.Server
     ├─ conversations → Runner / Session
     └─ 公共服务 → Agents / LLM / Skills / Commands
```

旧 Templ / HTMX / POST / SSE 页面体系、页面插件、模拟原型与迁移施工文档已经删除，不维护双轨。

## 已完成能力

- `PreToolUse` Hook 首版：全局与项目命令配置、设置页编辑和排序；工具参数校验后、实际执行前按顺序检查静态和 MCP 工具。空输出放行，明确 `deny` 回给模型；故障提示并放行，停止 Run 会取消 Hook。项目配置按工作区真实路径和整份文件摘要信任，外部修改后暂停，页面可重新确认。Hook 在宿主运行，信任不跟踪脚本内容；运行提示不进账本。设置页已由用户截图验收。
- 权限规则与接口：新增纯计算包 `kernel/permissions`，支持四档模式翻译、额外权限判断、受保护元数据目录和本次批准合并，并定义人／模型共用的 Reviewer 契约。Policy 仅保存无限制标记、可写根、联网标记。SessionSettings 持久保存 `permissionMode`，旧文件缺字段默认 Ask for approval，未知模式报错；设置更新省略模式时保留，分叉复制，子会话继承父 Run 快照。Go／TS 契约同步；模式菜单和审批服务见下一项。
- 审批：独立 `kernel/approvals` 服务管理命令额外权限申请、补丁目录申请与本次授权。支持人工、常规 LLM、Jev；设置页全局选择审核方式，LLM 复用已配置模型与思考档位，Jev 密钥与 LLM 共用 config.yaml。信息不足、故障或 Jev 批准置信度不足转人工；停止取消、重连恢复人工待审批，批准仅影响本次操作。新消息标记真实用户来源，子 Agent 委派追溯父用户授权；旧来源无法确认的会话转人工。常规 LLM 实际使用与设置页截图已由用户验收。
- Agent 沙箱：Runner 将本轮 Policy 经 Loop 传给 Tool；命令走 AgentExec，补丁走 AgentApplyChanges 的内部写入助手。Linux 共用 bwrap + seccomp，根只读、授权根可写、元数据保护、禁网与宿主 socket 阻断已接通；旧进程不接受权限更新。macOS Seatbelt 已在 macOS 27.0 arm64 实机验证。用户文件和终端保留直接入口。缺少系统启动器或不支持的策略明确失败；Windows 受限执行暂未实现，Full Access 可用。MCP 不在此沙箱覆盖范围，其独立权限边界见下文。
- 项目与会话：原生目录选择、按工作区分组、新建或复用空会话、切换及自动命名。
- 聊天：文字与图片、实时输出、直接 Steer、停止父子任务、历史、刷新、重连与后台重启恢复。
- 上下文引用：主聊天 `@` 搜文件／目录、文件树右键、Monaco 选区右键及完整助手回答文字选区添加。助手片段通过选区旁的紧凑框添加可选评论，确认后独立编号，编号在删除引用或下一条消息发送成功前持续锚定原选区，悬停先显示评论、再以分隔线显示原文。其他标签可整块删除，代码选区可展开预览并保留添加时未保存内容。引用随会话草稿保留，允许仅带引用发送，失败保留，确认只清本次提交的附件；用 v1／v2 版本标记的普通文本落账，用户历史、Steer、刷新和分叉共用还原逻辑。
- 呈现：Snapshot 与事件共用一份投影；工作过程按账本结构同级展示说明、思考、工具和子任务回报，工具详情局部展开；支持 Markdown 清洗、最终回答及失败／停止状态。流式增量每帧最多刷新一次，只重绘变化的 Turn；辅助区和隐藏子任务不随聊天文字重绘。
- 设置：模型与思考、SessionSettings、Agent 增删改及删除保护。
- 操作：上下文用量、回答分叉、Skill 候选、命令候选与 compact。
- 子任务：固定允许主会话第 0 层 → 子任务第 1 层 → 孙任务第 2 层，第 2 层不能继续委派。父子 Session 平级存储；Task v1 仍只保存直属关系、委派说明和稳定任务名，深度由关系计算，轮次、结果与通知从子会话和直属父账本投影。任一层 Bot 卡片都可在辅助面板打开独立工作页，标签显示完整路径；支持实时过程、续聊／Steer、递归停止、空闲设置和 Diff 审查，多标签共享一条 WebSocket 并各自订阅，关闭视图不停止任务。
- 辅助工作区：公共壳统一管理可登记视图的混合标签、`+` 菜单和面板关闭；当前登记文件、Diff 审查、终端和只能从聊天子任务卡片打开的子任务视图。左右侧栏、辅助区和文件树均可调宽。
- 编辑器视图：自己的第二行工具栏管理路径、保存状态和文件树折叠；右侧文件树与左侧 Monaco 支持多文件标签、UTF-8 高亮编辑、700ms 自动保存、`Ctrl+S`、未保存关闭确认及按项目保留本页草稿。
- 文件同步：打开文件和展开目录通过 `fs/watch` 监听；干净文件自动重载，外部冲突保留本地草稿并提供重载／覆盖，删除或保存失败不丢缓冲区。聊天中的本地文件链接可打开编辑器并定位行列。
- Diff 审查：`apply_patch` 每次真实落盘后按 Run 实时聚合净变化，聊天显示文件数与增删行；审查页按需读取单文件前后内容，支持单双栏 Monaco Diff、变更列表调宽，以及 Run 结束后的版本保护单文件撤销。
- 浏览器终端：右侧辅助区支持多个真实 Git Bash 标签；基于 xterm.js、ConPTY 与 machine-local PTY，支持交互输入、ANSI、窗口 Resize、退出状态和关闭时终止进程树。终端标签在切换或隐藏辅助区时保持运行，断开 Client 后统一清理。
- 界面菜单：普通页面统一使用受控右键菜单，Monaco 使用自身 Command 菜单；浏览器尚未接入。
- 视觉系统：暖灰侧栏、白色内容区、细线分隔与蓝色焦点已应用到聊天、设置和辅助工作区；保留 Ginto／Noto Sans SC 字体，正文调整为 14px、导航 12px，收紧行高与间距，最大 660px 阅读列及面板拖动保留。设置页精简常驻说明、区分导航图标，Hooks 以可折叠列表展示。Monaco 保留 JetBrains Mono 并同步亮暗配色；等待用户运行页面截图验收。
- 核心 Tool：`exec_command` / `write_stdin` 统一使用 Bash，支持普通管道与 PTY、增量输出、持续输入、轮询和 `Ctrl+C`；`apply_patch` 支持 Codex 格式的多文件新增、修改和删除。旧 `read/write/edit/bash` 已移除，已有 Agent 启动时幂等迁移到新 Tool。

## 后台边界

- 智能审批本轮验收：`make agent-check` 通过，另对 approvals 与 subagents 执行针对性 race。覆盖两种适配、拒绝、失败/低置信度转人工、取消、全局设置持久化与在途快照，以及真实用户与模型委派来源区分。Jev 独立探针扩为 15 例，实测 15/15、平均 483ms、错误放行 0；探针提示词与正式适配不同，此结果不是正式安全校准。用户随后确认常规 LLM 实际使用与智能审批设置页截图验收；Jev 的生产安全准确率仍未校准。

- appserver 是入口直接组装的接入层，只管理静态页面、协议、连接、订阅与清理，不拥有业务状态。
- appserver 的类型化方法、单 Client 连接和系统目录选择已分成三个内部职责；同一 Session 的写请求排队，不同 Session 并行，Stop 直接执行。
- conversations 负责创建、发送、Steer、停止、快照、分叉和命令准入。
- Runner 负责运行、草稿、事件、取消与收尾；模型输出及整批工具结果组成不可插入的步骤，Steer 与协作回报只在随后检查点按序落账。连接断开不停止已接受的 Run。
- Stop 是独立的 Context 取消；它不进账本，并可取消仍在等待检查点的 Steer。
- 耐久消息先落账再发布；增量只进入运行投影。生成、草稿和最终 Entry 共用同一个 Entry.ID。
- 每个 Session 同时只有一个活 Run；运行状态、最近用量和 Diff 摘要保存在 `runs.json`，Diff 正文独立压缩保存；未完成运行在重启后标记 interrupted，不自动续跑。
- Client 只保存服务端投影和草稿、主题、折叠等临时界面状态，不成为业务事实来源。
- appserver 已显式接入同一份 machine 文件能力，提供读取、受版本保护的保存、目录、路径搜索、元数据与监听 RPC；监听复用统一订阅和断线清理。`fs/searchPaths` 搜索工作区相对路径，逐层遵守 `.gitignore`、跳过 `.git` 与符号链接，最多 50 项；前端 200ms 防抖并忽略迟到结果。
- appserver 通过连接级 `command/exec` 系列 RPC 管理 UI 终端；进程按 `ConnectionID + processId` 隔离，输出实时通知 Client，断线、取消或服务关闭都会终止并等待进程收尾。它不进入会话操作或 Session 账本。
- machine-local 持有文件与长期进程的平台能力；长期进程按 Harness Session 隔离，进程 ID 已交付后可跨 Turn 存活，关闭时终止进程树并等待读取与回收完成。`apply_patch` 写入前完整计算所有目标，匹配失败或文件并发变化时不覆盖，I/O 中途失败准确返回已提交前缀。Agent Tool 只转换参数和呈现结果。

## 构建与启动

正式命令在仓库根目录执行：

```sh
make run          # 构建 React 并启动，自动打开浏览器
make build        # 产出 .build/harness
make test         # 前后端、契约、网络、vet 与相关 race
make agent-check  # Agent 日常快速回归：构建一次，其余常用检查并行
```

Vite 构建产物位于 `clients/web/dist/`，由 Go embed 进入二进制但不提交 Git。`make run` 与 `make build` 会先生成它；不再读取 `harness.yaml`，监听地址固定为 `127.0.0.1:8888`。

前端开发页仍可在 `clients/web` 运行 `npm run dev`，默认使用 `http://127.0.0.1:5173`，同源 `/rpc` 代理到正式后台的 `8888` 端口。

## 数据与配置

数据根目录固定为 `~/.harness`：

```text
~/.harness/
├─ config.yaml
├─ agents/<agent-id>.json
├─ skills/<skill>/SKILL.md
├─ system/skills/<skill>/SKILL.md
├─ sessions/<session-id>/{meta.json,messages.jsonl,settings.json,runs.json,diffs/<run-id>.json.gz}
├─ subagents/tasks/<task-id>.json   # 仅父子关系与委派说明；状态和结果来自子会话
└─ mcp.json
```

本机模型 Provider 仍在 `~/.harness/config.yaml` 配置。machine-local 的用户入口直接操作本机；Linux Agent 受限入口使用 `/usr/bin/bwrap`（需描述符挂载、seccomp 与 user namespace 支持）；编辑器 RPC 单文件限制 2 MiB，保存使用 SHA-256 版本避免覆盖已变化内容。
Windows 启动继续要求 Git Bash；Agent 长期进程与 UI 终端 PTY 均使用 `charmbracelet/x/xpty`，共享 machine-local 的进程树清理能力，但各自管理身份与生命周期。它们只存在当前 Harness 进程内存，重启后失效。
`~/.harness` 固定使用本机文件存储；Session、Runner、Agent、LLM、MCP 用户配置、Skill 用户目录与内置 Skill、Subagents 共用 `persist` 的可靠文件读写，不提供 SQLite 切换。

## 后续范围

- Wails 包装正式 React 页面与唯一后台的启动／退出。
- 通用服务端反向请求；人审批通过专用订阅与回答接口恢复。
- 有副作用操作的业务 ID 防重及完整多 Client 协调。
- 辅助工作区中的浏览器。

这些能力尚未实施，不应提前增加兼容层或通用框架。

## 已知未验证

- 沙箱不提供硬链接别名隔离；保护目录内部的细粒度子路径授权暂时拒绝。Linux 临时占位在正常退出时清理，宿主被强杀或断电可能留下空目录；不自动删除来源不明的目录。macOS Seatbelt 已在 macOS 27.0 arm64 实机验证，其他 macOS 版本与 Intel 机器未运行。Windows 沙箱仍待后续实现。智能审批只审核原本需要申请的操作；

- 上下文引用各入口、助手选区浮层／悬停预览、中文输入法和亮暗／窄屏布局待用户 `make run` 截图验收；按最新要求不新增 UI 自动化测试。
- Windows 原生目录选择器仍需在交互式 Windows 桌面验收。
- Subagent 工作页的窄面板布局、嵌套打开 `主会话 › 孩子 › 孙子`、点击去重、关闭父标签后孙子保持订阅及实机流式交互仍需浏览器验收。
- 前端主包与延迟加载的 Monaco 包超过 Vite 默认 500KB 提示；编辑器及 Worker 均为本地资源并按需加载，当前不影响启动和离线运行。
- Windows 管理员环境下 `TestSpawnInitialPersistFailureConsistency` 不能用目录只读位制造写入失败，导致 `make agent-check` 的既有 subagents 测试失败。
- `TestProcessOutputIsIncremental` 已改为信号文件同步两段输出，不再要求首包在固定等待窗口内到达；此前 Mac / Windows 的时序失败记录保留在下方，改后的测试尚待这两个平台重跑。

## 本次验证

- `PreToolUse` 首版：静态/MCP 工具统一埋点、顺序与拒绝、故障放行、项目配置变更后重新信任、取消中的命令，以及 Hook 设置 RPC 的实际注册/调用均通过定向测试；`make agent-check` 全通过。设置页由用户截图验收，用户确认全局日志 Hook 实际运行成功。

- 修复 Agent 批量写入的两个锁问题：路径 key 只规范化一次，获取锁时不再解析且等待可取消；Full Access 也通过可终止的文件助手写入，不持有 machine 服务锁做文件 I/O。新增一组回归用例覆盖等待期间符号链接变化、取消路径锁等待，以及 FIFO 阻塞时其他命令仍可启动、文件助手可取消。machine-local、applypatch、appserver、Runner 的 race 与相关包 vet 通过；本次 `make agent-check` 仍因 npm `EALLOWREMOTE` 退出。

- Linux Agent 沙箱：真实 bwrap 集成验证项目内写入、项目外/符号链接/保护目录拒绝、Read Only 与 Full Access、TCP/Unix stream/Unix datagram 阻断、补丁整批权限预检与部分提交、Session 归属和 PTY 后续交互。`go test ./kernel/... ./plugins/... ./products/... ./appserver/...` 与对应 vet 通过；machine-local、补丁/命令 Tool、Runner、ReAct 的相关 race 通过。后台包在 macOS arm64、Windows amd64 交叉编译通过；Linux arm64 machine-local 编译通过，非 Linux 未做运行验收。
- 本次 `make agent-check` 在前端 npm ci 阶段因 `EALLOWREMOTE`（禁止下载锁文件中的远程包）退出；没有改动依赖配置。`go build ./cmd/harness` 因缺少 `clients/web/dist` 无法完成，因此前端和完整可执行文件构建未验收；写入助手已通过同样内部入口的测试可执行文件实际运行验证。

- 权限规则、设置持久化与更新、分叉与子会话模式继承测试通过；permissions、persist、appserver、harness Product、subagents 的 race 与相关包 vet 通过。`make agent-check` 在前端依赖阶段因环境缺少 npm 退出；补跑 `make agent-go` 时其余 Go 包测试通过，只有 `clients/web` 与 `cmd/harness` 因缺少前端 `dist` 嵌入产物无法编译，因此该目标后续的全量 vet 未执行。前端构建与 TS 检查未执行，不能视为全量验收通过。
- 助手消息片段引用扩充了现有核心用例：7 项覆盖 v1／v2 往返、特殊字符、坏段回退、去重与确认清理，全部通过；前端生产构建通过，只有既有 Vite 大包提示。未新增 UI 自动化测试，实机视觉交互留给用户验收。
- 上下文引用只保留两组核心测试：后端路径搜索的 `.gitignore`／匹配／截断，前端引用文本的转义往返／无效段回退／去重与确认清理。Go 编译、相关包 vet、TS 契约检查和前端生产构建通过；只有既有 Vite 大包提示。未新增 UI 自动化测试，也未执行包含全量 UI 测试的 `make agent-check`。
- Subagent 两层委派、恢复图校验、直属回报、递归停止、准备期停止竞态、停止后逐层续聊、隐藏会话隔离和真实 WebSocket 嵌套订阅测试通过；TS 契约、RPC 类型检查、64 项前端测试、生产构建和 Go vet 通过。`make agent-check` 仅有已记录的 machine-local `TestProcessOutputIsIncremental` 首次输出为空，在普通与 race 检查中失败；未修改该模块。
- Subagents 生命周期收为 `ctx / work / shutdown`：取消状态统一用于关闭准入，一份计数等待调用与后台退出，`sync.OnceValue` 复用关闭结果；取消与解除订阅函数只由关闭函数持有。补强了创建阻塞期间并发 Close 不得提前返回的测试。
- 本次关闭基线与修改后的相关测试、Subagents 及产品/Runner/委派工具 race、Go vet、前端构建、64 项前端测试和 TS 契约检查通过。`make agent-check` 未全通过：本机 `plugins/machine/local.TestProcessOutputIsIncremental` 在普通与 race 检查均因首次输出为空失败，单独 race 复核仍失败；未修改该模块。
- TypeScript 编译、生产构建、TS 契约检查及 64 项前端测试通过。
- `apply_patch` 的真实 Delta、Run 内聚合、缓存与失效测试通过；Diff 正文恢复、分叉复制、单文件撤销和后续修改冲突测试通过。
- 受 Runner 新增 machine 依赖影响的 subagents 与 ReAct 测试脚手架已补齐真实 machine-local；对应 race 检查通过。
- `make agent-check` 的前端、契约、Go 编译与其余测试通过；最终只剩上述 Windows 管理员环境下既有的只读位权限测试失败。
- 浏览器终端的 appserver、machine-local、TypeScript 契约、前端 RPC 与 60 项前端测试通过；`make agent-check` 仍只剩上述既有 Windows 管理员权限测试失败。
- Subagent 工作页的归属隔离、旧任务兼容、父会话结束后续聊、运行中 Steer、单独停止、空闲设置、稳定点击身份、共享连接独立订阅和 Diff 接口检查通过；`make agent-check` 仍只剩上述既有 Windows 管理员权限测试失败。
- 未调用付费模型或修改用户会话。

### 第 3 阶段验证（2026-09-22）

- `go test ./kernel/... ./plugins/... ./products/... ./appserver/...` 与对应 `go vet` 通过。核心验证覆盖审批批准/拒绝/取消/关闭、重复回答、真实 WebSocket 重连、Linux 本次目录及联网授权与其他目录继续只读、权限说明前缀保留及截断后的基线。
- approvals、Runner、appserver、exec、applypatch 的 race 通过；machine-local 的 race 首次仅新增网络用例因测试助手退出超过 1 秒而失败，延长该用例等待后单独 race 通过，其余用例在首次检查中通过。
- `make agent-check` 在 npm ci 阶段因 `EALLOWREMOTE`（远程依赖下载被禁用）退出。未修改依赖限制；前端类型检查、构建与完整 CLI 构建未验收。网页不做自动化操作，由用户运行 `make run` 截图验收。

- 第 3 阶段补验收：修正前端两个 SessionView 测试样本缺少 permissionMode 的类型错误。npm 仓库配置修正后，`make agent-check` 全部通过：前端生产构建、71 项前端测试、TS 契约与 RPC 类型检查、全量 Go 测试/vet 及目标 race。此前 EALLOWREMOTE 与缺少 dist 的构建阻塞已解除；实际网页交互仍由用户截图验收。

- 输入框工具栏调整为添加图片 → 权限模式 → Agent；权限菜单复用公共按钮与上弹菜单，窄布局保留图标入口，顶栏移除原选择框。`make agent-check` 通过；视觉效果由用户运行 `make run` 截图验收。

- 待审批界面改为替换主聊天底部输入框，复用输入框外观；直接展示命令和申请权限，右下角拒绝或允许一次，多项申请逐条处理，结束后恢复原草稿。`make agent-check` 通过；未操作浏览器，视觉交互由用户截图验收。

- 权限菜单通过 `permissions/modes` 获取后端模式清单，名称统一为只读、请求批准、智能审批、完全访问。智能审批暂不可用，前端禁选且后端拒绝切入；已有权限模式 ID 与持久化格式不变。补充现有设置校验测试，`make agent-check` 通过；视觉交互由用户验收。

- 审批框收紧间距与按钮尺寸，理由和命令限高滚动，权限摘要保留在标题；完整目录范围与来源折叠到详情，详情入口与操作按钮同排。`make agent-check` 通过，视觉效果待用户截图验收。

- 输入框的权限、Agent、模型三个菜单入口移除向下箭头，保留原点击交互。`make agent-check` 通过。

### macOS 沙箱实现（2026-09-22）

- 新增 Darwin 平台的 Seatbelt 启动方案，使用固定系统 sandbox-exec、内存 SBPL 与独立路径参数；命令与文件修改助手复用同一入口。处理系统顶层路径别名、授权根及祖先保护、元数据目录保护、网络开关与特殊 fcntl 限制；失败不降级。
- `make agent-check` 通过；macOS arm64 / amd64 的 machine-local 测试可执行文件与完整 Harness 可执行文件交叉编译通过。未在 Linux 上执行 Mac 二进制，不能视为实机限制验证通过。
- 已编写一组 Mac 核心集成测试：目录写入与保护、符号链接、只读与本次授权、文件助手、PTY、取消及 TCP / Unix socket。换到 Mac 后执行 `go test ./plugins/machine/local -run TestDarwinAgentSandbox -v`；再用 `make run` 验收实际 HTTPS 与审批授权。测试不访问外网。
- macOS 27.0 arm64 实机执行 `go test ./plugins/machine/local -run '^TestDarwinAgentSandbox$' -v -count=1` 通过。首次运行发现 Unix socket 测试地址超过系统长度限制，改用短临时路径后，文件边界、保护目录、符号链接、本次授权回落、文件助手、PTY、取消及 TCP / Unix socket 网络开关全部通过。
- 本机 `make agent-check` 的前端生产构建、71 项前端测试、TS 契约与 RPC 检查及其余 Go 包通过；普通和 race 检查均命中已记录的 `TestProcessOutputIsIncremental` 首包为空时序问题，因此全量目标退出失败。
- 用户手工验收通过项目内免审批写入、项目外拒绝与单次授权、真实 HTTPS 单次联网、只读、完全访问和交互式 PTY。单次授权不会延续到下一次操作；未申请额外权限的联网直接由规则拒绝。停止和刷新审批本轮未重复验证，属于平台无关的审批生命周期与界面行为。
- Agent PTY 的 Tool 结果在进入模型与聊天卡片前转换为稳定纯文本，处理回车覆盖、退格、清行和 ANSI 控制序列；machine 仍保存原始增量字节，右侧 xterm 终端不受影响。回归用例覆盖 SSH 密码提示的同一行重复重绘，exec Tool 测试通过；`make agent-check` 的前端、契约和其余 Go 检查通过，普通与 race 仍只命中已记录的 `TestProcessOutputIsIncremental` 首包时序问题。

- 增量输出测试改为读取第一段后创建临时信号文件，再输出第二段；关闭 Bash 启动配置加载，允许输出分片与空轮询，并用整体超时兜底。只修改测试，不改变进程等待语义。本次 Linux 上该用例连续 10 次 race 通过，`make agent-check` 全部通过；Mac / Windows 尚待重跑。

### 智能审批修正（2026-09-22）

- 移除运行权限提示中“模型审核尚未接入”的过期说明，明确智能审核、转人工及现有权限内操作免审核的边界。
- apply_patch 在审批前检查拟申请的已有目录是否真正覆盖目标；缺失受保护目录且父目录授权无法覆盖时，明确要求用户先创建目录，再申请显式权限，避免无效审批后才由 machine 拒绝。没有增加自动创建受保护目录的能力。
- 现有测试文件补充三类受保护目录存在/缺失的核心回归；定向单测与 `make agent-check` 通过。

### 智能审批审查修复（2026-09-23）

- 授权来源追溯按原始 Entry ID 全局去重，防止跨轮次跟进时把父会话旧授权重新追加到子会话新限制后；用户后来重复的同文消息仍保留。现有核心测试增加跨轮次顺序与同文新消息回归。
- 删除临时 TypeSafe 探针脚本和两份测试结果，硬编码测试密钥不进入提交。
- `go test -race ./kernel/approvals` 与 `make agent-check` 通过。用户报告 Jev 实际批准、拒绝及沙箱对照测试通过；普通 LLM 的真实使用后来也已由用户确认。

### MCP 权限边界（2026-09-23）

- 项目 MCP 在连接前由底部审批框确认；确认记录按项目真实路径及解析后的有效配置摘要保存在 `~/.harness/approvals/mcp-trust.json`。拒绝后本轮只保留全局工具；变更项目配置或其环境变量后，旧快照拒绝后续调用，重启时读取并重新确认。
- 只读模式不发现或调用 MCP Tool；请求批准逐次人审，智能审批逐次由所选 LLM/Jev 审核并在不确定时转人工，完全访问直接调用。MCP Server 仍在宿主或远端运行，MCP 审批不修改文件与网络沙箱权限。
- MCP 核心测试覆盖连接前确认、拒绝、逐次批准/拒绝、只读与完全访问，以及配置变更阻断；真实 WebSocket 审批快照通过契约验证。相关 Go 测试和 MCP/approvals race、前端构建与 TS 检查通过。`make agent-check` 的前端测试在本机触发文件监听 `EMFILE`；使用 `CHOKIDAR_USEPOLLING=1 npm --prefix clients/web test` 顺序重跑，71 项通过。网页视觉与真实 MCP Server 交互待用户 `make run` 验收。
- 审查修复：项目确认展示完整命令参数与工作目录或完整 URL，按钮明确表示按配置版本记住信任；缓存连接复用前核对工作区真实路径，符号链接改指向即拒绝旧连接。定向 MCP race、TypeScript 与轮询模式前端测试通过；本轮并行 `make agent-check` 另有一次 `fs/watch` race 用例失败，单项重跑通过。

## 架构精简：主干迁移（2026-09-24）

- `products/harness` 已迁入 `kernel/conversations`，按会话协调发送、设置与命令；跨会话不再共用发送锁，Stop 仍独立执行。
- 删除 Host 服务表及服务装配 Plugin。入口显式创建依赖与登记扩展，长期资源取得后立即安排收尾；实际 Tool / Loop / Skill / MCP 实现保留。
- Runner 普通运行与压缩共用结束记录和通知流程；压缩准备去掉与设置重复的工作区、模型和思考档位字段。普通取消草稿和失败压缩的处理区别保留。
- 主聊天、子任务聊天与子任务 Diff 共用订阅生命周期；草稿文字、图片、引用与编辑版本合为一份会话记录，保留迟到确认保护。UI 样式未改变。
- 本批复用原有测试并删除 Host 注册专属测试；只新增一条跨会话阻塞回归。`make agent-check` 的前端构建、71 条前端测试和契约检查通过；Go 阶段发现本次方法重命名遗漏，修正后 `make agent-go agent-race`（全量 Go 测试、vet、指定包 race）通过。运行界面仍由用户验收，未提交。
- 领域存储格式迁移、编辑器文件生命周期提取尚未实施，不计入本批完成范围。
