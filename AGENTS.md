# AGENTS.md

压缩会话、换人、新开对话：先读这篇。方案细节：`learn/插件设计书.md`。Cordis 对照：`learn/项目心得.md`。不要先翻 DSH / pi 源码。`reference/` 只是别人怎么做，不是我们的方案。

冲突：铁律以这篇为准，产品形状以设计书为准。

`learn/项目心得.md` 默认不要改。用户叫写才写。

---

## 原则

### 心智模型

**进程是一张服务表。聊天是桌上的一摊产品，不是根。**

能换的做成登记处，别人来填；不能换的做成一整份服务。一场对话是一次 `Runner.Send` / `Run`：怎么想是 Kind（要编译），这份对话怎么配是 Setup（人设、工具名单、模型、思考档位、工作区文件夹）。账本只记说过的话。屏幕听这一轮 `Emit`，不听账本。插件要编译进去；配置只决定这次启不启动，不排顺序，不热加载新代码。

动手之前先问：这是桌上新的一摊，还是聊天里的一份数据、一种画法？工作区、自定义模式、轨迹页，全是聊天产品，不改内核形状。不要为了对齐 DSH 往内核加插槽。

```
Host（桌子）
  ├─ 聊天：Setup + Session + Runner.Run(sessionID, 话) → Agent.Run → llm.Stream
  ├─ 狼人杀：验架构，v1 不实现。自己的棋盘，可以 Get Runner
  ├─ 多机器人：验架构，v1 不实现。自己的房间；每个机器人一本 session
  └─ 电影：验架构，v1 不实现。只占 HTTP，零依赖对话
```

`Run` 不加 `appName` / `user_id`。用户拦在 HTTP；产品是桌上另一摊。

### 铁律

1. 参考 DSH / pi，**不照抄**。Go。静态编译。无动态插件。无热加载。无 AI 自改代码。
2. **根是插件宿主，不是 Runner。** 聊天只是宿主里的一摊。电影播放器可以零依赖对话。
3. 对话：`Runner.Run` → `Agent.Run` →（仅 LLM 类）`llm.Stream`。Runner 在 Agent 外面。换 loop = 换 Agent Kind，不是换 Runner。
4. Agent 是一种程序，不是一场焊死的对话。session 在 `Run` 的参数上。接着问 = 闲着再 `Run`（FollowUp）。还在转时插一句 = `Runner.Steer`。不要 `Chat.Followup`，不要 Inbox。
5. 自定义：Kind（开发者代码，要编译）vs Setup（用户数据：人设、模型、思考档位、已有工具名）。用户不热加载 Go。Setup 是这本聊天配置的唯一事实来源；改完下一轮 `Run` 读取新快照。
6. 提示词是登记处 `prompts`，只拼文本。系统提示词是来填槽的插件。人设是 Setup，每次 Assemble 现取。工具名单 `Get("tools")` 现取。不要代收 schema。
7. Session **只记对话**，可以分叉。todo / 审批 / 游戏状态放插件自己的结构体。别往账本塞。
8. 屏幕听这一轮 Run（`Emit`；浏览器用 SSE）。不听账本。喇叭就这一个，插件不要各搞各的。耐久事件先 `Append` 再给屏幕，失败则终止 Run。
9. 前端：内核一份，表面按端 enable。Web 和 webview 同一套 templ；TUI 另画；ACP 是管子不是画面。HTML 壳 8 个洞（侧栏/浮层给桌上另一摊）。请求 POST，通知 SSE。v1 不用 WebSocket。

---

## 怎么讲解

讲解用项目词：登记处 / 插槽 / 活对象。不要编「袋子」「缝」「椅子」。

### 话术

一个插件可以三件都做，不必须只做一件。

1. 注册服务 — 表上多一个键，分两种形态：

   整份服务（A）  `RegisterService("llm", client)`  一个就能 Stream 的对象
   登记处  （B）  `RegisterService("tools", 空名单)`  ← 等人来做第 2 步

2. 填充插槽 — 往 B 的名单里塞一条
3. 回调

`Resolve("llm").Stream(...)` 是在用整份服务。
`Resolve("tools").Register(bash)` 才是填插槽。

表上的值一律叫 **服务**。不叫能力。A/B 只描述服务形态，不套活对象。

类型放哪的「数据 / 契约 / 活对象」是文件分类。这里的三样是 Host 上的东西：

| 叫什么 | 在哪 | 例子 |
|---|---|---|
| **服务** | 表上一个键，`Resolve` 拿到 | `llm`、`tools`、`sessions` |
| **插槽条目** | 往 B 里塞的一条 | `Tool`、Kind、prompt 段 |
| **活对象** | 用出来之后才有 | 一本 `Session`、一场 `Runner` 里的 `Run` |

填槽 ≠ 使用。`Register` 是填；`Call` / `Run` 才是用。B 没有 `Create` 出来的活 Tool。活着的 `Run` 在 `Runner.live`。

**契约** = 定义者包里写的：服务方法 / 插槽条目形状 / 活对象方法。Go 就是类型所在的包。Host 的表运行时无类型；怎么调只在定义者包。消费者 import 定义者，不 import 提供者。

### 四问

一次只讲一个。先说是不是插件，再说已经有了还是计划。

```
【它是什么】     一句话
【提供能力】     别人 Resolve 到什么
【使用能力】     它 Resolve 哪些已有的
【填充插槽】     填哪个 B；没有就写「不填」
```

登记一种 Kind 再加一问：**`Run` 是否取尽 `Steers`？** 不取尽不准 Register。

不是插件（比如 Setup）：它是什么、干什么、谁产生/谁用。先人话，再代码名。不把没做的说成已经有了。

---

## 怎么写代码

### 规范

#### 类型放哪

三种名字，三个位置。**导出 ≠ 进 `types.go`。**

```
数据     Message / Input / Setup / Node     别人要造、要读的形状
契约     Plugin / Persistence / Tool        别人要遵守或要填的口
活对象   Host / Store / Session / Client    挂在 Host 上的那份东西
```

```
数据、契约  →  定义者包的 types.go
活对象      →  跟它方法同一个文件（即使导出也不进 types.go）
包内私货    →  谁用放谁那
```

`plugin.go` 只做 Start / Close，不放业务类型。包很小、契约自己就是文件名时（host 的 `Plugin`），不必硬拆 `types.go`。

每个 `type` 注释第一句标明种类：`数据` / `契约` / `活对象`。

```go
// 数据。Runner 已准备好的本轮输入。
type Input struct { ... }

// 契约。按 session 读写 Setup。
type Setups interface { ... }

// 活对象。挂在 Host 上的那份 LLM 客户端。
type Client struct { ... }
```

接口默认没有。现在就有两种实现要换，或别人来填槽（B），才做接口。不为测试、不为「以后也许有 mock」预建。

谁先说出这个词，类型就在谁那。别人 import，不要抄一份。

类型跟定义者走：块在 `session`，goai 流事件留在 goai，Setup / Invocation 在 `kinds`。Setups 接口跟 Setup 走（kinds）；For/Put 的实现在 persist 包，Host 键是 `setups`。persist 可以 import kinds，只为存 Setup。`Persistence` 接口仍然只谈 Tree。

#### 一个包怎么拆

打开一个包，先填四格：表上有键吗？A 还是 B 还是库？契约有几份、各给谁？本包不做什么？

一个领域一份服务。服务里面按职责拆文件。**真能换的才做插槽**（B）。别把小能力都 `RegisterService`。

```
Plugin    Name + Start + Close。Start 里挂上，Close 里拆
插槽      B 的 Register 口。不是第三种插件
```

默认同包分文件，不先拆子包。看文件名就是菜单。不要 `manager` / `util` / `common`。

A（Runner）：

```
kernel/runner/
  plugin.go    Start：Resolve 依赖，New，RegisterService("runner", r)
  types.go     跨包的数据、契约
  run.go       type Runner struct { live ... }  活对象，和 Run
  steer.go     Steer
  spawn.go     Spawn / Wait / Stop
```

B 登记处（tools）：

```
kernel/tools/
  plugin.go    挂上空登记处
  types.go     Tool 接口、Schema（契约 / 数据）
  registry.go  Register / 按名 Call
```

填充者：

```
plugins/bash/
  plugin.go    Resolve("tools")、Resolve("world")，Register(bash)
  bash.go      Call 里 world.Spawn
```

三件都满足才加一层目录：能独立替换；边界稳定；不会绕一圈转发。

```
优先：领域包 → 章节文件 → 函数
```

不因为文件长或「以后也许有用」而拆包。不建空的 `tui/`、`acp/`。

#### import

```
cmd      → kernel / surface / plugins
plugins  → kernel（只定义者）
surface  → kernel
kernel   不得 import plugins、surface
定义者   不得 import 填充者
```

提供者和消费者都 import 定义者，彼此不 import。

### 风格

- 标识符英文。注释中文。commit 双语。
- 可读性优先，不炫技。
- `err := f()` 和 `if err != nil` 分成两行。
- 每包一个主要公开构造入口；构造只校验依赖和组装。
- 不要没有主人的 `utils.go` / `helpers.go` / `common.go`。

---

## 怎么组装

### 仓库

两层半。看 `kernel/` 就是 Host 那张表。一个 `go.mod`。

```
cmd/harness/
  main.go            读 yaml，按顺序 Start
  harness.yaml

kernel/
  host/              桌子。Plugin、RegisterService、Resolve、Close 倒序
  persist/           Persistence + Setups；jsonl.go / sqlite.go（配置选，不是 enable 插件）
  session/           Session / 块 / Message；Store
  world/             World；local.go / e2b.go（配置选，不是 enable 插件）
  llm/               plugin.go + Client / models.json；直接调 goai
  tools/             空登记处 + Tool
  prompts/           空登记处 + Section / Assemble
  kinds/             空登记处 + Agent / Invocation / Setup
  runner/            整份 A；live
  http/              路径登记处
  pages/             8 个洞

surface/
  web/               v1。templ + htmx + SSE

plugins/
  bash/              填 tools + prompts
  prompt-harness/    填系统提示词那段
```

`tui/`、`acp/` 后期真写再加目录，**不要先建空文件夹**。

### 启动

```yaml
persist: jsonl
world: local
enable: [web, bash, prompt-harness]
```

```
main:
  host.New()
  必装：persist（挂 sessionPersistence + setups）→ world → session → llm → tools → prompts → kinds → runner → http → pages
  再按 enable：plugins/* 、 surface/*
  Close 倒序
```

顺序只出现在 `cmd/harness`。yaml 不能重排。jsonl / local 是 A 包内部选文件，不出现在 enable。内核每次都 Start，不进 `enable`。

没有 `service.go`。Host 上那份对象就是结构体本身。服务名硬编码字符串。取出时泛型 Resolve。对不上就是组装错误。

```go
host.RegisterService("runner", r)
r, err := host.Resolve[*runner.Runner](h, "runner")
```

一份 A 用结构体 `Resolve[*llm.Client]`；能换的 A 用接口 `Resolve[persist.Persistence]`。

### 装上和拆掉

- B 的 `Register` 返回幂等 `unregister`。
- `Install` 中途失败：已 Start 的倒序拆。
- 谁开长期资源，谁关。
- 一次铺一个领域，测过再下一个。不顺手加功能，不改已经公开的能力名。
- 可选插件用配置 `enable`；内核名单和 Start 顺序留在 Go。v1 改配置后重启。不热加载未编译的包。
