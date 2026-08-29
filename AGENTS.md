# 怎么写代码

动手按这篇。方案：`learn/插件设计书.md`。铁律：`learn/给后来的AI.md`。话术：`learn/项目心得.md`。

一个领域一份服务。服务里面按职责拆文件。**真能换的才做插槽**（B）。别把小能力都 `RegisterService`。

```
Plugin    Name + Start + Close。Start 里挂上，Close 里拆
插槽      B 的 Register 口。不是第三种插件
```

没有 `service.go`。Host 上那份对象就是结构体本身。

能力名硬编码字符串。取出时泛型 Resolve。对不上就是组装错误。

```go
host.RegisterService("runner", r)
r, err := host.Resolve[*runner.Runner](h, "runner")
```

标识符英文。注释中文。commit 双语。

## 仓库怎么摆

两层半。看 `kernel/` 就是 Host 那张表。一个 `go.mod`。

```
cmd/harness/
  main.go            读 yaml，按顺序 Start
  harness.yaml

kernel/
  host/              桌子。Plugin 接口、RegisterService、Resolve、Close 倒序
  persist/           Persistence + Setups；jsonl.go / sqlite.go / setups.go（配置选，不是 enable 插件）
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

类型跟定义者走：块在 `session`，goai 流事件留在 goai，Setup / Invocation 在 `kinds`。Setups 接口跟 Setup 走（kinds）；For/Put 的实现在 persist 包，Host 键是 `setups`。persist 可以 import kinds，只为存 Setup。`Persistence` 接口仍然只谈 Tree。

## 一个包里面

默认同包分文件，不先拆子包。

### 类型放哪

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

Host 取出：一份 A 用结构体 `Resolve[*llm.Client]`；能换的 A 用接口 `Resolve[persist.Persistence]`。

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

看文件名就是菜单。不要 `manager` / `util` / `common`。活着的 `Run` 在 `Runner.live`。

## import 箭头

```
cmd      → kernel / surface / plugins
plugins  → kernel（只定义者）
surface  → kernel
kernel   不得 import plugins、surface
定义者   不得 import 填充者
```

## 启动

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

顺序只出现在 `cmd/harness`。yaml 不能重排。jsonl / local 是 A 包内部选文件，不出现在 enable。

## 什么时候才拆子包

三件都满足才加一层目录：能独立替换；边界稳定；不会绕一圈转发。

```
优先：领域包 → 章节文件 → 函数
```

不因为文件长或「以后也许有用」而拆包。不建空的 `tui/`、`acp/`。

## 装上和拆掉

- B 的 `Register` 返回幂等 `unregister`。
- `Install` 中途失败：已 Start 的倒序拆。
- 谁开长期资源，谁关。
- 一次铺一个领域，测过再下一个。不顺手加功能，不改已经公开的能力名。
- 可选插件用配置 `enable`；内核名单和 Start 顺序留在 Go。v1 改配置后重启。不热加载未编译的包。

## 写法

- 可读性优先，不炫技。
- `err := f()` 和 `if err != nil` 分成两行。
- 每包一个主要公开构造入口；构造只校验依赖和组装。
- 接口只为现在的边界，不为未来预建。每个 `type` 注释第一句标明：数据 / 契约 / 活对象。
- 不要没有主人的 `utils.go` / `helpers.go` / `common.go`。

## 讲一个插件时四问

一次只讲一个。先说是不是插件，再说已经有了还是计划。

```
【它是什么】     一句话
【提供能力】     别人 Resolve 到什么
【使用能力】     它 Get 哪些已有的
【填充插槽】     填哪个 B；没有就写「不填」
```

登记一种 Kind 再加一问：**`Run` 是否取尽 `Steers`？** 不取尽不准 Register。

不是插件（比如 Setup）：它是什么、干什么、谁产生/谁用。先人话，再代码名。不把没做的说成已经有了。
