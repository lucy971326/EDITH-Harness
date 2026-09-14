# 项目状态

更新日期：2026-09-14

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
- 呈现：Snapshot 与事件共用一份投影；工作过程按账本结构同级展示说明、思考、工具和子任务回报，工具详情局部展开；支持 Markdown 清洗、最终回答及失败／停止状态。
- 设置：模型与思考、SessionSettings、Agent 增删改及删除保护。
- 操作：上下文用量、回答分叉、Skill 候选、命令候选与 compact。
- 子任务：父子 Session 平级存储；Task v1 只保存关系，轮次、结果与通知从子会话和父账本投影。
- 辅助工作区：公共壳统一管理可登记视图的混合标签、`+` 菜单和面板关闭；当前登记文件与 Diff 审查视图，后续终端或浏览器无需修改公共壳。左右侧栏、辅助区和文件树均可调宽。
- 编辑器视图：自己的第二行工具栏管理路径、保存状态和文件树折叠；右侧文件树与左侧 Monaco 支持多文件标签、UTF-8 高亮编辑、700ms 自动保存、`Ctrl+S`、未保存关闭确认及按项目保留本页草稿。
- 文件同步：打开文件和展开目录通过 `fs/watch` 监听；干净文件自动重载，外部冲突保留本地草稿并提供重载／覆盖，删除或保存失败不丢缓冲区。聊天中的本地文件链接可打开编辑器并定位行列。
- Diff 审查：`apply_patch` 每次真实落盘后按 Run 实时聚合净变化，聊天显示文件数与增删行；审查页按需读取单文件前后内容，支持单双栏 Monaco Diff、变更列表调宽，以及 Run 结束后的版本保护单文件撤销。
- 界面菜单：普通页面统一使用受控右键菜单，Monaco 使用自身 Command 菜单；浏览器与终端尚未接入。
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
- appserver 已显式接入同一份 machine 文件能力，提供读取、受版本保护的保存、目录、元数据与监听 RPC；监听复用统一订阅和断线清理。
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
Windows 启动继续要求 Git Bash；长期 PTY 使用 `charmbracelet/x/xpty`。长期进程只在当前 Harness 进程内存中存在，重启后旧 `process_id` 失效。
`~/.harness` 固定使用本机文件存储；Session、Runner、Agent、LLM、MCP 用户配置、Skill 用户目录与内置 Skill、Subagents 共用 `persist` 的可靠文件读写，不提供 SQLite 切换。

## 后续范围

- Wails 包装正式 React 页面与唯一后台的启动／退出。
- 服务端反向请求与待回答恢复。
- 有副作用操作的业务 ID 防重及完整多 Client 协调。
- 辅助工作区中的浏览器和终端。

这些能力尚未实施，不应提前增加兼容层或通用框架。

## 已知未验证

- Windows 原生目录选择器仍需在交互式 Windows 桌面验收。
- 前端主包与延迟加载的 Monaco 包超过 Vite 默认 500KB 提示；编辑器及 Worker 均为本地资源并按需加载，当前不影响启动和离线运行。
- Windows 管理员环境下 `TestSpawnInitialPersistFailureConsistency` 不能用目录只读位制造写入失败，导致 `make agent-check` 的既有 subagents 测试失败。
- `TestProcessOutputIsIncremental` 的 80ms 首次等待在 Windows 负载较高时可能早于 Git Bash 首包输出结束；同次 race 检查通过，属于阶段 A 既有时序测试不稳定。

## 本次验证

- TypeScript 编译、生产构建、TS 契约检查及 58 项前端测试通过。
- `apply_patch` 的真实 Delta、Run 内聚合、缓存与失效测试通过；Diff 正文恢复、分叉复制、单文件撤销和后续修改冲突测试通过。
- 受 Runner 新增 machine 依赖影响的 subagents 与 ReAct 测试脚手架已补齐真实 machine-local；对应 race 检查通过。
- `make agent-check` 的前端、契约、Go 编译与其余测试通过；最终只剩上述 Windows 管理员环境下既有的只读位权限测试失败。
- 未调用付费模型或修改用户会话。
