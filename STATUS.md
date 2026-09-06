# 项目状态

更新日期：2026-09-06

## 现在是什么

Harness 已完成阶段 1、2、3，以及阶段 4 的五个页面插槽基础设施：它是可启动的 Web 聊天产品，具备项目/会话管理、真实 Runner 调度与 SSE 流式界面。

插件已按契约所有者归档：`plugins/kernel` 放内核服务实现与登记处填充物，`plugins/web` 放 Web 产品及其页面插槽填充物；Chat 是 `plugins/web/chat` 下的 Web 产品。

```text
浏览器 POST → Chat → Runner → Agent 设置 → React Loop → LLM / Tools
浏览器 SSE  ← Chat ← Runner 稳定事件 ← 完整消息先落账
```

运行 `go run ./cmd/harness` 会读取 `harness.yaml`，组装完整服务链，并在 `http://127.0.0.1:8888` 打开 Chat。

## 已完成

### 阶段 1：Web 基础

- `surface/web`：HTTP Server、产品/路由登记处、templ 通用页面壳。
- `plugins/web/chat`：Chat 产品注册及页面。
- 本地嵌入 HTMX `2.0.10`、SSE 扩展 `2.2.4`；Tailwind 编译到 `surface/web/static/site.css`。

### 阶段 2：项目与会话

- `SessionMeta` 独立保存 `ID / Title / CreatedAt`；元数据是空会话存在的依据。
- 新会话显示「新对话」；首条用户消息落账后自动改名。
- Chat 按 `Workspace` 分组展示项目与会话；项目不是独立数据。
- Win / macOS / Linux 原生目录选择；取消返回 Chat，真实错误才显示。

### 阶段 3：真实聊天

- 启动链已完整组装：`persist → session → llm → machine-local → tools → events → loops/react → skills → skills-builtin → skills-filesystem → agents → commands → runner → subagents → subagent-tools → chat-service → compact → web → chat → chat-composer-skills → chat-composer-commands → panel-demo`。
- 每轮生成 `RunID`，写入本轮耐久消息与 SSE；History Snapshot 和 SSE 共用 RunView reducer / `paint()`。同一 Run 默认合并为一张助手卡，只有耐久 Steer 才切成前后片段；落账完成不会让实时卡片跳位。
- 时间线的耐久顺序使用 `Entry.Seq`；运行中卡片用 `AfterEntrySeq` 定位，Run 内按 `StepSeq / BlockSeq` 排列，工具结果按原始调用块回填。
- Chat 支持普通发送、停止和 Steer；已移除 FollowUp 入口与等待队列，每个活 Run 只执行当前一轮。
- Steer 在接受时立即落账；工具被停止时也会补齐「已取消」结果，不留下悬空工具调用。
- `Runner.Start` 同步占住 Session，并在启动 goroutine 前完成一次 Agent / Skill 准备；准备错误直接返回 HTTP，之后在 Runner 管理的 goroutine 运行；`Runner.Close` 会取消并等待仍在运行的 Run。
- `kernel/chat` 的 `ChatService` 是聊天业务入口：创建或复用空会话、下一轮设置校验与启动、Steer/停止、快照、分叉、命令与 Agent/Skill 查询都经它完成；它不拥有账本、Run 或事件登记处。Chat Web、分叉动作、输入候选与 Agent 设置页不再直接使用这些内核服务。
- Runner 对界面只发布稳定事件：开始、文本/推理 Delta、工具开始/完成、用量、耐久消息、结束状态。
- 每次 SSE 重连重新同步 History；耐久快照会覆盖已排队的旧 Delta，慢客户端被断开，不会阻塞 Run。
- 模型与思考档位是独立选择框；每次普通发送前两者必选，换模型会清空档位并影响下一轮 Run。
- `models.json` 为每条模型手写 `contextWindow` 和 `vision`；`Models()` 带给 Chat，模型下拉用 `data-context-window` / `data-vision` 挂上。当前 DeepSeek 两条窗口 100 万、不看图；Google `gemini-3.5-flash-lite` 窗口 104 万、能看图。
- Chat 在模型选择旁画用量球：每次模型调用结束后，React 读 `ChunkFinish.Usage`，Runner 发 `usage` 事件，走现有 SSE，`chat.js` 更新。已用 = InputTokens + CacheReadTokens。不进账本，刷新后从 0% 开始。
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

- `surface/web/runview` 提供 Templ 运行视图和公共浏览器 reducer；它统一处理 ChatService History Snapshot、SSE Delta、Step / Block 排序、Tool 回填、工作流、Markdown 与展开状态。
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

- 必装 `kernel/subagents` 整份服务，启动顺序为 Runner → Subagents → ChatService；提供 Options / Spawn / Send / List / Wait / Stop / StopFamily。
- 使用父 Run 快照继承设置，独立创建子 Session；关系先保存，随后创建会话与设置、启动 Runner。子会话不进入普通聊天列表或空会话复用。
- `subagents/tasks/<task-id>.json` 保存版本化任务、逐轮 RunID / 状态 / 最终正文位置和通知集合；重启保留历史，未完成轮次标记中断，不自动执行。
- 单任务状态与写盘串行，旧轮完整收尾后才能开新轮；关闭取消服务上下文并等待在途启动和运行回调。持久化失败由调用、查询、等待及关闭明确报告。
- 已通过全量 Go 测试、相关包 race、vet 与 diff 检查，覆盖极快完成、旧轮收尾、启动中关闭、实际写盘失败和恢复。
- 平台工具进展见下节；父账本通知、等待插话和 Chat 父子停止尚未接入，StopFamily 与并发派生的完整协调留在第 5 步。

### 子会话委派第 3 步：工具入口

- 新增 `plugins/kernel/tools/subagents`，在 Subagents 之后静态安装，向现有 tools 登记处填入 options / spawn / send / list / wait / stop 六个 `subagent_*` 工具；插件不拥有后台资源，子 Run 仍由 Subagents 与 Runner 管理。
- 身份取可信调用上下文，不允许模型参数指定父会话、RunID 或工作区；服务拒绝越权及二层委派。Agent / 模型 / 档位分别继承父 Run 快照，显式覆盖独立校验。
- 工具沿用普通 Tool 权限，安装不替已有 Agent 勾选；需在 Agent 工具清单手动启用。options 返回现有 Agent 设置与模型清单，不新增“用途”字段或第二份名单。
- send 只允许非空文字，忙时追加、闲时开启下一轮；可能已经接收的错误不会自动重发。list 返回含逐轮最终正文的 TaskView 查询投影，正文仍只存在子账本。
- wait 接入单孩子当前轮的基础等待，默认 60 秒、允许 0～60 秒，超时不停止孩子；返回状态与结果位置。通知 ID、多个孩子等待、用户插话提前唤醒及自动投递留待第 4 步。
- 已通过全量 Go 测试、相关包 race、vet 和 diff 检查；覆盖六工具调用、权限、参数、身份隔离、继承与覆盖、忙闲追加、保存失败反馈和取消。尚未做真实模型端到端验收（第 6 步），未推进第 4 步。

## 验证

- `go test ./...` 通过。
- `go vet ./...` 通过。
- `git diff --check` 通过。
- `node --test plugins/web/chat/static/test/sidepanel.test.js` 通过。
- `node --check surface/web/static/runview.js` 与 Chat 私有脚本通过。
- 已做真实浏览器页面与布局检查。

## 下一步

```text
阶段 4 扩展（插槽填充物）：
  └─ sidepanel：真实文件树与文件查看面板

阶段 5：完整验收
  ├─ 轨迹页
  └─ 产品切换、SSE 重连、慢客户端、取消、关闭与浏览器验收
```

未完成工作只写 `docs/plan/future_plan.md`；做完的计划删除。UI 规范见 `WEB_UI.md`，已完成事实只写本文件，产品形状写入 `docs/设计书.md`。

## 运行前提

- Go 1.25。
- 修改 `.templ` 后运行 `go tool templ generate`。
- 修改样式后运行 `npm run web:build`；首次需要 `npm install`。
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
