# 项目状态

更新日期：2026-09-19

## 当前形状

Harness 已完成 React Web 迁移，正式入口只有一条：

```text
http://127.0.0.1:8888/    React 静态页面
ws://127.0.0.1:8888/rpc   WebSocket + JSON-RPC 2.0

React Client
     ↓
appserver.Server
     ├─ HarnessProduct → Runner / Session
     └─ 公共服务 → Agents / LLM / Skills / Commands
```

旧 Templ / HTMX / POST / SSE 页面体系、页面插件、模拟原型与迁移施工文档已经删除，不维护双轨。

## 已完成能力

- 项目与会话：原生目录选择、按工作区分组、新建或复用空会话、切换及自动命名。
- 聊天：文字与图片、实时输出、直接 Steer、停止父子任务、历史、刷新、重连与后台重启恢复。
- 上下文引用：主聊天 `@` 搜文件／目录、文件树右键与 Monaco 选区右键添加；输入框上方标签可整块删除，选区可展开预览并保留添加时未保存内容。引用随会话草稿保留，允许仅带引用发送，失败保留，确认只清本次提交的附件；用带版本标记的普通文本落账，用户历史、Steer、刷新和分叉共用还原逻辑。
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
- 视觉系统：按确认的 HTML 原型落地居中阅读布局，正文与输入框共用 660px 列宽和水平留白；右侧默认约四分之一窗口宽度，可拖动，Diff 文件列表可折叠。统一亮暗语义 Token，本地打包 Ginto、JetBrains Mono，中文统一回退 Noto Sans SC；等待用户截图验收。
- 核心 Tool：`exec_command` / `write_stdin` 统一使用 Bash，支持普通管道与 PTY、增量输出、持续输入、轮询和 `Ctrl+C`；`apply_patch` 支持 Codex 格式的多文件新增、修改和删除。旧 `read/write/edit/bash` 已移除，已有 Agent 启动时幂等迁移到新 Tool。

## 后台边界

- appserver 是 Host 外的接入层，只管理静态页面、协议、连接、订阅与清理，不拥有业务状态。
- appserver 的类型化方法、单 Client 连接和系统目录选择已分成三个内部职责；同一 Session 的写请求排队，不同 Session 并行，Stop 直接执行。
- HarnessProduct 负责创建、发送、Steer、停止、快照、分叉和命令准入。
- Runner 负责运行、草稿、事件、取消与收尾；模型输出及整批工具结果组成不可插入的步骤，Steer 与协作回报只在随后检查点按序落账。连接断开不停止已接受的 Run。
- Stop 是独立的 Context 取消；它不进账本，并可取消仍在等待检查点的 Steer。
- 耐久消息先落账再发布；增量只进入运行投影。生成、草稿和最终 Entry 共用同一个 Entry.ID。
- 每个 Session 同时只有一个活 Run；运行状态、最近用量和 Diff 摘要保存在 `runs.json`，Diff 正文独立压缩保存；未完成运行在重启后标记 interrupted，不自动续跑。
- Client 只保存服务端投影和草稿、主题、折叠等临时界面状态，不成为业务事实来源。
- appserver 已显式接入同一份 machine 文件能力，提供读取、受版本保护的保存、目录、路径搜索、元数据与监听 RPC；监听复用统一订阅和断线清理。`fs/searchPaths` 搜索工作区相对路径，逐层遵守 `.gitignore`、跳过 `.git` 与符号链接，最多 50 项；前端 200ms 防抖并忽略迟到结果。
- appserver 通过连接级 `command/exec` 系列 RPC 管理 UI 终端；进程按 `ConnectionID + processId` 隔离，输出实时通知 Client，断线、取消或服务关闭都会终止并等待进程收尾。它不进入 Product、Host 或 Session 账本。
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

本机模型 Provider 仍在 `~/.harness/config.yaml` 配置。machine-local 直接操作本机文件和进程，没有沙箱与路径限制；编辑器 RPC 单文件限制 2 MiB，保存使用 SHA-256 版本避免覆盖已变化内容。
Windows 启动继续要求 Git Bash；Agent 长期进程与 UI 终端 PTY 均使用 `charmbracelet/x/xpty`，共享 machine-local 的进程树清理能力，但各自管理身份与生命周期。它们只存在当前 Harness 进程内存，重启后失效。
`~/.harness` 固定使用本机文件存储；Session、Runner、Agent、LLM、MCP 用户配置、Skill 用户目录与内置 Skill、Subagents 共用 `persist` 的可靠文件读写，不提供 SQLite 切换。

## 后续范围

- Wails 包装正式 React 页面与唯一后台的启动／退出。
- 服务端反向请求与待回答恢复。
- 有副作用操作的业务 ID 防重及完整多 Client 协调。
- 辅助工作区中的浏览器。

这些能力尚未实施，不应提前增加兼容层或通用框架。

## 已知未验证

- 上下文引用三个入口的实机交互、中文输入法和亮暗／窄屏布局待用户 `make run` 截图验收；按最新要求不新增 UI 自动化测试。
- Windows 原生目录选择器仍需在交互式 Windows 桌面验收。
- Subagent 工作页的窄面板布局、嵌套打开 `主会话 › 孩子 › 孙子`、点击去重、关闭父标签后孙子保持订阅及实机流式交互仍需浏览器验收。
- 前端主包与延迟加载的 Monaco 包超过 Vite 默认 500KB 提示；编辑器及 Worker 均为本地资源并按需加载，当前不影响启动和离线运行。
- Windows 管理员环境下 `TestSpawnInitialPersistFailureConsistency` 不能用目录只读位制造写入失败，导致 `make agent-check` 的既有 subagents 测试失败。
- `TestProcessOutputIsIncremental` 的 80ms 首次等待可能早于 Git Bash 首包输出结束；普通或 race 检查都可能因此失败，属于阶段 A 既有时序测试不稳定。

## 本次验证

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
