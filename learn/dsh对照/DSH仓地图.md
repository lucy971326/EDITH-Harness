# DSH 仓地图

这是 DeepSeek Harness 怎么装。**不是我们的方案。** 开工读 `learn/给后来的AI.md`。

防串包备忘，不是讲义。细节在 `core/`。

讲解 **DeepSeek 主干** 用这些词对照源码（不是我们 Go 的分层）：

| 我们的词 | 源码 |
|---|---|
| 登记处 Roster | `agents` / `AgentRegistry` |
| 工厂 Factory | `AgentFactory` / `setFactory` |
| 对话 Chat | 接口 `Agent`；实现 `ReactLoopAgent` |
| 票 Ticket | `AgentHandle` |
| 司机 Driver | 包 `agent-loop`，类 `AgentLoop`，键 `agentLoop` |
| 转圈 | Chat 的 kick/turn/step，不是第三个类型 |

核对：`reference/deepseek-harness/docs/index.md` 当索引 + `architecture.zh.md` / `capability-seams.zh.md` + `packages/` 源码。
冲突以源码为准。旁支只点名挂点，不展开。

---

## 仓怎么长

没有特权内核。运行中的 `dsh` 是一棵 Cordis 插件树。

```
空配置
  → profile 列出的组合包（先 base，再 web-app 或 headless）
  → profile 自己的 cordis.patch.yml
  → home 级 patch
  → --patch
```

- **profile**：`$DSH_HOME/profiles/<name>` 里的具名组装。发行版模板：`web`、`headless`。
- **组合包**：一份 `cordis.patch.yml` 补丁层。`dsh-base` 每份 profile 的第一层。
- 包按组放：`packages/<组>/<包>/`，npm 名仍是 `@deepseek-ai/dsh-<包>`。
- **没有** `packages/plugins/`。没有 VSCode 应用。入口只有 `apps/cli`（`dsh`）和 `apps/web`（浏览器壳，由 web profile 端出来）。

`packages/core/` **不是一个定义者**，是一组包。`llm` 不在 core。

---

## 用户消息从哪进

```
人 / 程序
  dsh --profile web        → 浏览器 → client/* RPC → host/apiproxy
  dsh --profile headless   → bundle/headless  runner
  ACP / SDK JSON-RPC       → 各自 session/prompt
        │
        ▼
  Get("agents").Create(...)     ← 登记处把活转给工厂（司机），返回票
  票.Chat.Followup(话)          ← Steer / Inject 同 inbox，唤醒不同
        │
        ▼
  Inbox 追加 + 敲门
        │
        ▼
  这台对话转圈
      Session.Append(turn/start …)     ← 日志唯一写入口
      systemPrompt.Assemble
      Session.DeriveMessages()         ← 问模型的历史从日志折
      agent/request → ctx.llm.stream
      Session.Append(assistant/chunk|message)
      Get("tools").Execute             ← 只调登记处方法
      Session.Append(tool/call|result)
        │
        ▼
  Broadcast("session/event")           ← 已经写进内存了
  flush → session/flush                ← 持久化把缓冲落盘（本包不写磁盘）
```

编程入口永远是 `ctx.agents`（import `dsh-agent`），**不** import `dsh-agent-loop`。

`Session.Append` 是写日志的唯一 API。对话主干是 loop 在写；goal / compaction / title 等也可以往**同一本**日志 Append 领域事实。持久化 **Subscribe**，不写内存日志。

---

## 三层

```
① 主干（对话机器）
   sessions / agents（登记处） / 司机 / tools / systemPrompt / llm

② 能力缝（定义者 + 可换提供者 + 面向模型的 tool-* 消费方）
   shell / fs / subprocess / sandbox / terminals / web / skills / subagents / …

③ 表面（谁把话送进 Followup、谁画 session/event）
   web：host/apiproxy + client/ui-*
   headless：一次性 runner
   ACP / SDK：协议桥
```

扩展挂在已有缝上，不改司机。新模型可见输入必须先成为一条会话日志。

---

## 主干服务（必认）

| 键 | 谁 Provide | A/B | 登记的是什么 | 谁填 / 谁用 |
|---|---|---|---|---|
| `sessions` | `core/session` | B | 活 `Session`（只追加日志） | 无工厂。司机 Create 时 `new`。**不落盘** |
| `agents` | `core/agent` | B | 活 Chat + **工厂插槽** | 司机填工厂。Create 转给工厂，返回票 |
| `agentLoop` | `core/agent-loop` | A | 司机另 Provide 的服务（组装用） | 填工厂；实现 Chat。入口仍是 `agents.Create` |
| `tools` | `core/tools` | B | 插槽条目 `Tool`（不是活对象） | `tool-*` Register；对话转圈时 Execute |
| `systemPrompt` | `core/system-prompt` | B | 段 / 变量 / 工具名单提供方 | 工具插件、persona、tools 包、司机填变量；对话 Assemble |
| `llm` | `llm/llm` | B | 适配器 | `llm-deepseek` / `llm-pi-ai` 登记；对话转圈时 `stream` |
| `agentDefaultModel` | `core/agent-default-model` | A | 没选模型时用谁 | headless / apiproxy 入口读 |

`scope/` 是库，无 ctx 键。按 agent 过滤登记，不是服务。

`agent-tool-presentation` 不占键：preset 行，填 `tools.presentAs`（native / code / both）。

```
Get("agents")
  ├─ factory          ← 插槽。司机填一次
  └─ live[id] → Chat ← 活对象。id = session.id
         Followup / Steer / Inject
         inbox 是结构体，实现在 agent/src/inbox.ts

Get("sessions")
  └─ live[id] → Session  ← 活对象。契约+实现都在 core/session
         Append / DeriveMessages / fork（旁支）
```

---

## 别把两个 session 目录混了

| 路径 | 键 | 干什么 |
|---|---|---|
| `packages/core/session` | `sessions` | 内存里的只追加日志 |
| `packages/session/*` | `sessionPersistence` 等 | 落盘、投影、标题、遥测 |

jsonl / sqlite **继承并 Provide** `sessionPersistence`（A，组合包选一个），自己听 `session/event` + `session/flush`。

loop 也听 `session/event`，只为投影动态上下文（`runtime-context.ts`），**不是写日志**。

---

## 能力缝怎么挂（只到槽）

典型三身份：定义者包 Provide 空缝 → 提供者 Provide/登记后端 → `tool-*` Get 缝、再 `tools.Register`。

| 缝（ctx 键） | 定义者 | 常见提供者 | 面向模型的填充者 |
|---|---|---|---|
| `shell` | `shell/shell` | bash-local / bash-sandbox / pwsh-* | `tool-bash` / `tool-pwsh` |
| `fs` | `fs/fs` | fs-local / fs-sandbox / fs-e2b | `tool-fs` |
| `subprocess` | `subprocess` | local / e2b | bash、PTY、LSP、进程外 subagent 都经它 spawn |
| `sandbox` / `sandboxPolicy` | `sandbox/*` | sandbox-local | 执行器包装 argv |
| `terminals` | `terminal` | terminal-bash | `tool-terminal` |
| `codeRuntime` | `code-runtime` | worker-thread | `tools` 在 code 模式下消费（`run_code`） |
| `subagents` | `subagent` | spawn/fork 进程内，acp/sdk/codex/claude-code | `tool-subagent*`、`tool-ralph` |
| `jobs` | `jobs` | jobs-local | `tool-jobs`；bash/PTY/subagent 登记后台活 |
| `web` | `web` | search-* / fetch-http | `tool-web` |
| `skills` | `skill` | filesystem / badge | `tool-skill` |
| `lsp` | `lsp` | lsp-local / stdio | `tool-lsp` |
| `workflowEngine` | `workflow` | worker-thread | `tool-workflow` / `tool-ralph` |
| `compaction` | `compaction` | compaction-basic | 无面向模型工具；听压力事件 |
| `approval` | `user-approval` | 监听器当回答方（含 ACP） | tools / tool-bash 门禁 |
| `commands` | `commands` | command-* | **给人**的斜杠命令，不进模型 |
| `userQuestions` | `user-questions` | UI 回答方 | `tool-ask-user` |
| `settings` / `credentials` | 各自定义者 | *-file / *-local | llm 适配器、apiproxy |
| `storage` / `storageDomain` | storage | json / sqlite | workspace、feedback（非会话 KV） |
| `workspaceRegistry` | `workspace` | 自己 | apiproxy |
| `agentPresets` | `preset/agent-presets` | 发现目录 | 创建期挂到 `agent.ctx` |
| `goals` / `planMode` | goal / plan-mode | 自己 | 命令 + 工具；状态仍在会话日志 |

`tool-bash` = A 缝 `shell` 的消费者 + B `tools` 的填充者。

---

## 事件三域（选错域是改错地方）

| 域 | 名字 | 干什么 |
|---|---|---|
| 会话**日志** | `turn/start`、`user/message`、`tool/result`… | 持久事实。模型可见即已记录 |
| 会话**实时** | `session/event`、`session/flush`、`session/created` | 刚 Append 完告诉别人；flush 并行等完、不否决 |
| Agent 实时 | `agent/*` | 进行中的活：inbox、pre-step、status。契约在 `agent/`，多半 loop 派发 |
| 能力实时 | `tools/*`、`llm/stream`、`fs/*`、`approval/request` | 门禁 / 适配，不必 import loop |

别混：

| | 谁的 | 干什么 |
|---|---|---|
| `session/event` | 实时回调 | 「日志又多了一条」 |
| `turn/start` | 日志里的一条 | 事实本身 |
| `tools/result` | `tools/` 实时 | 流水线刚结束 |
| `tool/result` | 日志里的一条 | 模型下次看得到 |

waterfall（要 `next()`）：`agent/pre-step`、`agent/request`、`llm/stream`、`tools/pre-execute|execute|post-execute`。
serial（无 next）：`agent/turn-stopping`。
parallel 等全部：`session/flush`。

---

## 应用入口

| 入口 | 路径 | 把话送进哪 |
|---|---|---|
| `dsh --profile web` / `dsh web` | `apps/cli` → base + web-app → `host/apiproxy` + `apps/web` | apiproxy → `agents` |
| `dsh --profile headless "任务"` | base + headless-runner | `agents.Create` + `Followup`，等 idle，打印，退出 |
| ACP | `packages/acp`；演示 `examples/acp-agent` | `session/prompt` → Followup |
| SDK JSON-RPC | `packages/sdk`；演示 `examples/jsonrpc-agent` | 同上 |
| 叶节点 examples | `examples/*` 的 `cordis.yml` 加载 `packages/examples/*-demo` | 演示组合，不是产品 API |

Web 画面对话：听 `session/event` / 投影，不自己写日志。

---

## 旁支（点名，不展开）

| 名字 | 挂在哪 |
|---|---|
| isolate / preset | `ctx.agentPresets`；preset 的 `cordis.yml` 挂到 `agent.ctx`，自带服务必须在 isolate realm 后 |
| Typert | `ctx.typert` + `ctx.typertGateway`；Host RPC 的类型图。agent/session 登记 lookup |
| ACP | `packages/acp` 吃 `ctx.agents`；`subagent-acp` 填 `ctx.subagents` |
| fork | `sessions.fork`；`subagent-fork-in-process` 用父日志前缀做子会话 |
| repair | `core/session/src/repair.ts`，崩溃尾巴补关闭；不是 ctx 服务 |
| code-mode | `tools` 的 mode / `presentAs` + `ctx.codeRuntime`；模型只直呼 `run_code` |
| MCP | `packages/mcp/mcp-client` 把外来工具 `Register` 进 `ctx.tools` |
| Agent Teams | `ctx.agentTeams`（实验，显式启用） |
| E2B | `ctx.e2b`；fs-e2b 与 subprocess-e2b 同一远程世界 |
| Ralph / workflow | `workflowEngine` + `tool-ralph`；不是 loop 模式 |
| HMR / Loader | vendored Cordis；动态插件、热重载。Go 移植不抄 |
| 自改 Cordis | `extensions/tool-cordis` + `dynamicCordisRunner`。Go 移植不抄 |

司机可替换。扩展依赖登记处包的事件和服务，不依赖司机包。

---

## 讲课时别混（这张表就是这份笔记的用处）

0. **我们的分层 ≠ 这份 DSH 地图。** 开工：Runner 在外、换 Agent 实现。下面 1–7 只防讲 DSH 串包。
1. 登记处 ≠ 工厂 ≠ 对话 Chat ≠ 司机。Inbox 是登记处包里的结构体。源码把后三样都叫 loop，讲解不要跟着叫。
2. `Tool` 是插槽条目，没有活 `Tool`。`Session` / Chat 才是活对象。
3. `packages/core/session`（内存）≠ `packages/session`（落盘）。
4. `tools/*` 实时 ≠ 日志 `tool/*`。
5. 定义者包可以有真代码（登记处、Inbox、Session 实现）。「定义者」不是「只有纸面」。
6. A/B 只描述**服务**：A = 整份被替换（`agentLoop`、`sessionPersistence` 后端、`shell`）；B = 登记处别人填（`agents`、`tools`、`systemPrompt`、`llm`）。活对象不是第三种服务。
7. 其余那么多包：填槽、听事件、画 UI。不是第二套内核。

---

## 五篇核对

`core/`：`agent.md` → `agent-loop.md` → `tools.md` → `session.md` → `system-prompt.md` → `llm.md`

机制仍对得上。讲解用词已改成登记处 / 工厂 / 对话 / 票 / 司机，源码名只作对照。

本次独立对照（我 + 子代理）分歧后再核：

- 司机包 **确实** Subscribe `session/event`，但是 `runtime-context` 投影用，不是写入口。
- `sessionPersistence` 是 A：jsonl/sqlite 继承定义者类再 Provide，不是往空登记处 Register。
- 仓库这份 reference 根目录没有 `package.json` / workspace 清单；以 `packages/` 组 README 为准。

---

## 以后从哪翻

| 要什么 | 看哪 |
|---|---|
| 架构总览 | `reference/deepseek-harness/docs/architecture.zh.md` |
| 键 → 定义者/提供者/消费方 | `reference/deepseek-harness/docs/capability-seams.zh.md` |
| 事件谁派谁听 | `reference/deepseek-harness/docs/event-producer-consumer.zh.md` |
| 某缝细节 | `reference/deepseek-harness/docs/subsystems/` |
| 组 → 包 → ctx 键 | `reference/deepseek-harness/packages/README.zh.md` 和各组 README |
| 主干精读 | `core/` |
| 我们的方案 | `learn/插件设计书.md` |
| Cordis 话术 | `learn/项目心得.md` |
