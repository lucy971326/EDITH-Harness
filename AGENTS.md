# AGENTS.md

压缩会话、换人、新开对话：先读这篇，再读 `STATUS.md`。方案细节：`docs/设计书.md`。Web UI 细则：`WEB_UI.md`。数据归属与运行文件：`DATA_MODEL.md`。Cordis 对照：`docs/reference/DSH项目心得.md`。不要先翻 DSH / pi 源码。`docs/reference/` 只是偶尔参考，不是我们的方案。

冲突：铁律以这篇为准，产品形状以设计书为准。

`docs/reference/` 默认不要改。用户叫写才写。

### 参考资料与查阅规则

`reference/` 目录存放第三方技术文档与参考项目：

1. **技术文档**：
   - 不懂 HTMX：查阅 `reference/htmx/`
   - 不懂 Templ：查阅 `reference/templ/`

2. **参考项目（DSH 与 pi）**：
   - 路径：`reference/deepseek-harness/` 和 `reference/pi/`
   - 两个参考项目的源码已建立 `codegraph` 索引。
   - **查阅优先级**：`项目文档 (docs) > codegraph 查源码 > 直接读源码`
   - 提醒：参考项目仅供对照设计思路，不照抄其具体实现。

---

## 原则

### 心智模型

**进程是一张服务表。聊天是桌上的一摊产品，不是根。**

能换的做成登记处，别人来填；不能换的做成一整份服务。一场对话是一次 `Runner.Send` / `Run`：怎么想是 Loop（要编译）；Agent 设置管 Kind、SystemPrompt、工具和 Skill 名单；这份会话怎么配是 SessionSettings（AgentID、模型、思考档位、工作区文件夹）。账本只记说过的话。屏幕听这一轮 `Emit`，不听账本。插件要编译进去；配置只决定这次启不启动，不排顺序，不热加载新代码。

动手之前先问：这是桌上新的一摊，还是聊天里的一份数据、一种画法？工作区、自定义模式、轨迹页，全是聊天产品，不改内核形状。不要为了对齐 DSH 往内核加插槽。

```
Host（桌子）
  ├─ 聊天：SessionSettings + Session + Runner.Run(sessionID, 话) → Loop.Run → llm.Stream
  ├─ 狼人杀：验架构，v1 不实现。自己的棋盘，可以 Get Runner
  ├─ 多机器人：验架构，v1 不实现。自己的房间；每个机器人一本 session
  └─ 电影：验架构，v1 不实现。只占 HTTP，零依赖对话
```

`Run` 不加 `appName` / `user_id`。用户拦在 HTTP；产品是桌上另一摊。

### 铁律

1. 参考 DSH / pi，**不照抄**。Go。静态编译。无动态插件。无热加载。无 AI 自改代码。
2. **根是插件宿主，不是 Runner。** 聊天只是宿主里的一摊。电影播放器可以零依赖对话。
3. 对话：`Runner.Run` → `Loop.Run` →（仅 LLM 类）`llm.Stream`。Runner 在 Loop 外面。换 loop = 换 Agent Kind，不是换 Runner。
4. Loop 是一种程序，不是一场焊死的对话。session 在 `Run` 的参数上。接着问 = 闲着再 `Run`（FollowUp）。还在转时插一句 = `Runner.Steer`。不要 `Chat.Followup`，不要 Inbox。
5. 自定义：Loop / Kind（开发者代码，要编译）vs Agent 设置（用户数据：SystemPrompt、已有 Tool / Skill 名）vs SessionSettings（会话数据：AgentID、模型、思考档位、工作区）。用户不热加载 Go。Agent 设置和 SessionSettings 都是实时事实来源；Runner 每轮读取一次。
6. 系统提示词属于 Agent 设置。`agents.Prepare` 现取选中 Skill 的摘要和本轮工作区，拼成最终 System Prompt；Runner 只拿成品交给 Loop。LLM 类 Loop 插件在 `Start` 时自己 Resolve `llm`、`tools`，运行时按本轮工具名单现取 schema。不要另设提示词登记处。
7. Session **只记对话**，可以分叉。todo / 审批 / 游戏状态放插件自己的结构体。别往账本塞。
8. 屏幕听这一轮 Run（`Emit`；浏览器用 SSE）。不听账本。喇叭就这一个，插件不要各搞各的。耐久事件先 `Append` 再给屏幕，失败则终止 Run。
9. 前端：内核一份，表面按端 enable。Web 和 webview 同一套 templ；TUI 另画；ACP 是管子不是画面。Web 内有产品、路由和少量页面插槽登记处；产品决定进入哪一摊，页面插槽只扩展某个产品内部。请求 POST，通知 SSE。v1 不用 WebSocket。
10. Web UI 的 Token、公共规则、图标、视觉方向与 JS 边界见根目录 `WEB_UI.md`。做 Web 页面、产品或页面插槽前必须阅读；其中 JS 规则同样是铁律。

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
| **服务** | 表上一个键，`Resolve` 拿到 | `llm`、`tools`、`sessions`、`machine` |
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

不是插件（比如 SessionSettings）：它是什么、干什么、谁产生/谁用。先人话，再代码名。不把没做的说成已经有了。

---

## 怎么写代码

### 规范

#### 结构体字段

数据结构体按数据契约定义。有行为的活对象，字段只表达三类东西：

1. **身份**：它是谁，例如 ID、名字、所属关系。
2. **配置**：它怎么运行，例如 `Config`。
3. **能力**：它靠什么做事，例如组合进来的服务、登记处、资源句柄。**字段即能力**，这也是 Go 组合的用法。

每个字段都必须说清属于哪一类。实现状态只在确实支撑这份能力时保存；能交给局部变量或标准库的，就不自己保存。

#### 类型放哪

三种名字，三个位置。**导出 ≠ 进 `types.go`。**

```
数据     Message / Input / SessionSettings / Node     别人要造、要读的形状
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

// 契约。按 session 读写 SessionSettings。
type SessionSettingsStore interface { ... }

// 活对象。挂在 Host 上的那份 LLM 客户端。
type Client struct { ... }
```

接口默认没有。现在就有两种实现要换，或别人来填槽（B），才做接口。不为测试、不为「以后也许有 mock」预建。

谁先说出这个词，类型就在谁那。别人 import，不要抄一份。

类型跟定义者走：块在 `session`，goai 流事件留在 goai，SessionSettings / Invocation 在它们的定义包。SessionSettingsStore 契约跟 SessionSettings 走；For/Put 的实现在 persist 包，Host 键是 `sessionSettings`。Agent 的纯数据与 Store 契约在 `agents/config`，persist 以 `agentStore` 挂同一份实现，避免 Agent 服务、Loop、Session、persist 的 Go import 环。`Persistence` 接口仍然只谈 Tree。

#### 一个包怎么拆

打开一个包，先填四格：表上有键吗？A 还是 B 还是库？契约有几份、各给谁？本包不做什么？

一个领域一份服务。服务里面按职责拆文件。**真能换的才做插槽**（B）。别把小能力都 `RegisterService`。

```
Plugin    Name + Start + Close。Start 里挂上，Close 只关自己开的长期资源
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
plugins/kernel/tools/bash/
  plugin.go    Resolve("tools")、Resolve("machine")，Register(bash)
  bash.go      Call 里 machine.Run
```

三件都满足才加一层目录：能独立替换；边界稳定；不会绕一圈转发。

```
优先：领域包 → 章节文件 → 函数
```

不因为文件长或「以后也许有用」而拆包。不建空的 `tui/`、`acp/`。

#### import

```
cmd             → kernel / surface / plugins
plugins/kernel  → kernel（只 import 自己填充的定义者）
plugins/web     → surface/web；确有业务需要时再 import kernel 定义者
surface         → kernel
kernel          不得 import plugins、surface
surface/web     不得 import Web 产品或页面插槽填充者
定义者          不得 import 填充者
```

提供者和消费者都 import 定义者，彼此不 import。

插件目录按它填充的契约所有者归档，不按技术名或插件大小平铺：

```text
plugins/kernel/...          内核服务的提供者、内核登记处的填充者
plugins/web/<product>/      填 Web products 登记处的产品
plugins/web/chat/<slot>/... 填 Chat 自己的页面插槽
plugins/web/settings/...    填 Web 公共 settings.section
```

Chat 当前是 Web 产品，因为它直接使用 `surface/web`、Templ、HTMX 和 SSE。TUI
将来自己画，只复用 Session / Runner 等内核；ACP 是协议桥，不画页面；桌面端用
WebView 承载同一套 Web，不复制一份 Chat。真实代码出现前不建空的 `plugins/tui`
或 `plugins/acp`。

### 风格

- 标识符英文。注释中文。commit 双语。
- 可读性优先，不炫技。
- 普通包只返回错误，不打日志。错误只在 main 等进程边界，或无法再返回错误的后台入口打印一次。
- 生产与测试代码一律把 `err := f()` 和 `if err != nil` 分成两行。
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
  persist/           Persistence + SessionSettingsStore + agents/config.Store；jsonl.go / sqlite.go（配置选，不是 enable 插件）
  session/           Session / 块 / Message；Store
  machine/           定义者，只放契约。不是 A。A 是提供者挂上之后表上那把键
  llm/               plugin.go + Client / models.json；直接调 goai
  tools/             空登记处 + Tool
  loops/             空登记处 + Loop / Invocation
  skills/            空登记处 + Skill 摘要
  agents/            Agent 设置服务
  runner/            整份 A；live

surface/
  web/               v1。产品 / 路由 / 页面插槽登记处；templ + htmx + SSE

plugins/
  kernel/
    machine/local/   本机。RegisterService("machine", …)
    loops/react/     必装默认 Loop；使用 llm、tools，填 loops
    tools/read/ write/ edit/ bash/   各填 tools
  web/
    chat/            Chat 产品；填 Web products 和 routes
      composer/demo/ 填 Chat composer.actions
      dock/demo/     填 Chat dock（仅测试安装）
      sidepanel/demo/ 填 Chat sidepanel
    settings/demo/   填 Web settings.section
```

`plugins/tui/`、`plugins/acp/` 后期真写再加目录，**不要先建空文件夹**。

### 启动

```yaml
persist: jsonl
machine: local
enable: [web, read, write, edit, bash]
```

```
main:
  host.New()
  必装：persist（挂 sessionPersistence + sessionSettings + agentStore）→ session → llm → tools → loops → react → skills → agents → runner
  必装提供者：yaml machine 选出的那一个，在 tools 前面 Start
  再按 enable：plugins/* 、 surface/*
  Close 倒序
```

顺序只出现在 `cmd/harness`。yaml 不能重排。jsonl / sqlite 是 persist 包内部选文件。machine 是 yaml 选提供者插件。两者都不进 `enable`。内核每次都 Start，不进 `enable`。

没有 `service.go`。Host 上那份对象就是结构体本身。服务名硬编码字符串。取出时泛型 Resolve。对不上就是组装错误。

```go
host.RegisterService("runner", r)
r, err := host.Resolve[*runner.Runner](h, "runner")
```

一份 A 用结构体 `Resolve[*llm.Client]`；能换的 A 用接口 `Resolve[persist.Persistence]`、`Resolve[machine.Machine]`。

### 装上和拆掉

- 启动时固定填入的 B（tools / loops / skills / HTTP）只 `Register`，随 Host 整体消失。运行中会离场的订阅（events）才返回幂等 `unregister`。
- `Install` 中途失败：已 Start 的倒序拆。
- 谁开长期资源，谁关。谁启动 goroutine 或服务器，谁在 `Close` 中停止并等它退出。为此必需的等待状态属于这份能力，不算过度设计。
- 一次铺一个领域，测过再下一个。不顺手加功能，不改已经公开的能力名。
- 可选插件用配置 `enable`；内核名单和 Start 顺序留在 Go。v1 改配置后重启。不热加载未编译的包。


## 用户要求！

1. 讲解东西能一句说明白就不要说很多，言简意赅,因为用户有轻微的阅读障碍，讲解要求通俗易懂但是不能是天马行空的比喻
2. 开发测试阶段如果用到了APIKEY直接硬编码且用用户给你的APIKEY，不要考虑什么安全问题！这不是你该考虑的事情，用户会兜底重置APIKEY的
