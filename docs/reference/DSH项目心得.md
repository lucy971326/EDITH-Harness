# Cordis 核心理念

开工先读仓库根 `AGENTS.md`，方案读 `设计书.md`。这篇是 Cordis → Go 对照，仅供偶尔参考。

插件就两种：函数 / 对象。`Service`、类插件 = 对象插件里 `Provide(name, this)`，不是第三种。

对应 Go：`Context` → `Host`，`Plugin` → `Start`，`Provide/Get` → `RegisterService/Resolve`。

---

## 1. Context：服务表

```
┌─ Context ─────────────┐
│  services["db"] = 对象  │
│  services["llm"] = 对象 │
└───────────────────────┘
```

```go
type Context struct {
    parent   *Context
    services map[string]any
}

func (c *Context) Provide(name string, value any) { c.services[name] = value }
func (c *Context) Get(name string) any            { return c.services[name] }
```

---

## 2. Plugin：装进去跑

```go
func DB(ctx *Context) { ctx.Provide("db", NewDB()) }

type API struct{}
func (API) Apply(ctx *Context) { ctx.Get("db").Query("...") }

ctx.Plugin(DB)
ctx.Plugin(API{})
```

函数：直接调。对象：调 `Apply`（Go 里是 `Start`）。

---

## 3. inject：没齐不跑（Cordis 有，Go 不迁）

```go
var API = struct {
    Inject []string
    Apply  func(*Context)
}{
    Inject: []string{"db"},
    Apply:  func(ctx *Context) { ctx.Get("db") },
}
```

名单上的名字都 `Provide` 了才调 `Apply`。Go 用启动顺序 + `Get` 失败返回 error，不做等待。

---

## 4. 登记可逆：关停才拆

`Start` 只挂上。回调进栈，`Close` 倒序执行。不是 `defer`（`Start` 返回就拆）。

```
Start:  Provide / Subscribe / OnCleanup(fn)  →  fn 入栈
... 程序继续跑 ...
Close:  fn3 → fn2 → fn1
```

```go
func (p Plugin) Start(host *Host) error {
    host.RegisterService("runner", svc)
    host.OnCleanup(svc.CloseAll) // 现在不调用
    return nil
}
// 之后 host.Close() 才 CloseAll
```

---

## 5. 事件：名字 → 回调列表

不是语言功能，不是 channel，没有人在 `for` 里空等。

```
listeners["ping"] = [fn1, fn2]

Subscribe  → append
Broadcast  → 当场 for 一遍，调完返回
```

```go
type Host struct {
    listeners map[string][]func(any)
}

func (h *Host) Subscribe(name string, fn func(any)) {
    h.listeners[name] = append(h.listeners[name], fn)
}

func (h *Host) Broadcast(name string, payload any) {
    for _, fn := range h.listeners[name] {
        fn(payload)
    }
}
```

`on/emit` = `Subscribe/Broadcast`。监听也进清理栈，插件卸掉一起拆。

---

## 6. 中间件：调 next 放行，不调即否决

`Broadcast` 只通知。中间件包一层，闭包。

```
log → auth → work
         ↘ 不调 next = 拦下，work 不跑
```

```go
func pass(msg string, next func(string) string) string {
    return next(msg)
}

func stop(msg string, next func(string) string) string {
    return "denied" // 不调 next
}
```

Cordis `waterfall` = Go `Intercept` / `RunChain`。

---

## 7. 三个身份：定义者 / 提供者 / 消费者

`Get` 只给对象，不给契约。方法、插槽形状、事件格式写在 **定义者包** 里。

提供者和消费者都 import 定义者，**彼此不 import**，才能换提供者。

定义者分两种（对应服务两种形态）：

- A 整份服务：只出契约，不装（`dsh-shell`）
- B 登记处：自己也是插件，`Provide` 空名单（`dsh-tools`）

```
消费者 ──import──► 定义者 ◄──import── 提供者
                    契约
         Get("shell") 调的是契约上的 run/start
```

| 身份 | 插件？ | 干什么 |
|---|---|---|
| 定义者 | A 否 / B 是 | 写契约：方法 / 插槽条目 / 事件名和 payload |
| 提供者 | 是 | 注册整份服务（A） |
| 消费者 | 是 | 按契约 Get 调用、填充插槽、Subscribe |

Go：定义者 = `interface` 所在的包。

## 8. A / B 极简

### A 整份服务（shell）

```go
// 定义者：不是插件
type Spec struct{ Cmd string }
type Result struct{ Out string }
type Shell interface {
    Resolve(cmd string) Spec
    Run(spec Spec) Result
}

// 提供者
type LocalBash struct{}
func (LocalBash) Resolve(cmd string) Spec { return Spec{Cmd: cmd} }
func (LocalBash) Run(spec Spec) Result    { return Result{} }
func (LocalBash) Start(host *Host) error {
    host.RegisterService("shell", LocalBash{})
    return nil
}

// 消费者：import 定义者，不 import LocalBash
func UseShell(host *Host) {
    sh := host.Resolve("shell").(Shell)
    _ = sh.Run(sh.Resolve("ls"))
}
```

### B 登记处（tools）

```go
// 定义者：也是插件。服务方法 + 插槽里一条的形状
type Tool struct {
    Name    string
    Execute func(args string) string
}
type Tools struct{ items map[string]Tool }

func (t *Tools) Register(item Tool)              { t.items[item.Name] = item }
func (t *Tools) Execute(name, args string) string { return t.items[name].Execute(args) }

func ToolsPlugin(host *Host) error {
    host.RegisterService("tools", &Tools{items: map[string]Tool{}})
    return nil
}

func ToolBash(host *Host) error { // 填充插槽
    host.Resolve("tools").(*Tools).Register(Tool{
        Name: "bash", Execute: func(string) string { return "ok" },
    })
    return nil
}

func Loop(host *Host) { // 调服务方法，不翻内部表
    _ = host.Resolve("tools").(*Tools).Execute("bash", "ls")
}
```

### 一身两役

`tool-bash` = A 的消费者 + B 的填充者。

```go
func ToolBash(host *Host) error {
    sh    := host.Resolve("shell").(Shell)
    tools := host.Resolve("tools").(*Tools)
    tools.Register(Tool{
        Name: "bash",
        Execute: func(cmd string) string {
            r := sh.Run(sh.Resolve(cmd))
            return r.Out
        },
    })
    return nil
}
```

---

# 我们怎么开工

完整方案：`设计书.md`。铁律、写法和话术：仓库根 `AGENTS.md`。

这篇只留 Cordis → Go。分层、账本、前端、Kind/Spec 以方案为准。

以前写过「session 不是插件」——作废。Session 是 A 服务。

