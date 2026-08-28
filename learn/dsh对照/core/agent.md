# 登记处（源码包名 `agent/`）

先忘掉源码里叠在一起的 agent。我们只认四样：

```
登记处 Roster     进程一份。表上的钥匙 agents
工厂   Factory    插槽：谁来造这场对话。只能一人
对话   Chat       一场一份。说话、转圈
票     Ticket     Create 的返回值。谁造谁拆
```

本包 = **登记处 + 纸面契约**。不造对话、不转圈。造和转在司机包（源码 `agent-loop/`）。

`Chat` 不是服务，也不是插槽。它是 `live` 里的一场对话。

---

## 对照源码（读文件时用）

| 我们的词 | 源码 |
|---|---|
| 登记处 Roster | `Agents` / `AgentRegistry`，键 `agents` |
| 工厂 Factory | `AgentFactory`，方法 `setFactory` |
| 对话 Chat | 接口 `Agent` |
| 票 Ticket | `AgentHandle`：`{ Agent, dispose() }` |
| 司机 Driver | 下一篇。源码类名 `AgentLoop` |

入口永远是登记处，不是司机：

```
Get("agents").Create(...)   → 票
票.Chat.Followup(话)
票.Dispose()                → 拆场
Get(id)                     → 只能说话，不能拆
```

---

## 两层

```
Get("agents")                         ← 登记处（B）
     │
     ├─ factory                       ← 工厂插槽。司机启动时填一次
     │
     └─ live["s1"] → Chat             ← 对话。Create 之后才有
              Followup("hi")          ← 调对话，不是调登记处
```

三份契约，都在本包：

| 契约 | 谁认 | 干什么 |
|---|---|---|
| 登记处的方法 | `Get("agents")` 的人 | 造 / 查 / 填工厂 |
| 工厂 Factory | 司机 | 插槽里那一条长什么样 |
| 对话 Chat | 拿到票或 `Get(id)` 的人 | Followup / Steer / Inject |

司机启动只填工厂。有人 `Create` 时，工厂 `new` 出一个认 Chat 的对象，放进 `live`，把票交给调用方。

`live` 是活着的对话名单，不是 `tools.Register` 那种插槽。

| 身份 | 谁 |
|---|---|
| 定义者 + B 插件 | 本包：写出契约；`Provide("agents", 空登记处)` |
| 填工厂 + 实现 Chat | 司机（`agent-loop/`） |
| 消费者 | Web / headless / ACP：import 本包，不 import 司机 |

---

## 1. 登记处

```go
type Factory interface {
    Create(opts CreateOpts) (*Ticket, error)
    Resume(opts ResumeOpts) (*Ticket, error)
}

type Roster struct {
    live    map[string]Chat // 对话名单，不是插槽
    factory Factory         // 插槽：只能一人
}

func RosterPlugin(host *Host) error {
    host.RegisterService("agents", &Roster{live: map[string]Chat{}})
    return nil
}

func (r *Roster) RegisterFactory(f Factory) error {
    if r.factory != nil {
        return errors.New("already set")
    }
    r.factory = f
    return nil
}

func (r *Roster) Create(opts CreateOpts) (*Ticket, error) {
    if r.factory == nil {
        return nil, errors.New("no factory")
    }
    return r.factory.Create(opts) // 转给工厂；没填就失败
}

func (r *Roster) Get(id string) Chat { return r.live[id] }
```

`RegisterFactory` = 填槽。`Create` = 把活转给工厂，返回票。

源码方法名是 `setFactory` / `create`，意思一样。

---

## 2. 对话 Chat（契约在这，函数体在司机包）

TS 的 interface 能写字段。Go 的 interface **只能写方法**，字段活在结构体上。

```go
type Chat interface {
    Inbox() *Inbox
    Followup(msg UserMessage)
    Steer(msg UserMessage)
    Inject(msg UserMessage)
}

type Ticket struct {
    Chat Chat
}

func (t *Ticket) Dispose() error {
    // 停转圈 → 从 live 拿掉 → 拆掉 Session → 拆掉 scope
    return nil
}
```

消费者：

```go
ticket, err := host.Resolve("agents").(*Roster).Create(opts)
if err != nil {
    return err
}
ticket.Chat.Followup(msg) // 说话
// ticket.Dispose()       // 拆场。Get(id) 没有这一步
```

票的本质：**所有权，不是第二场对话。** 说话走 Chat，拆场走票。

---

## 3. `Inbox` 就是个结构体

不是插件，不是服务。两列待办，实现就在 `agent/src/inbox.ts`。司机不重写它，只用。

```go
type Inbox struct {
    nextTurn []UserMessage // Followup
    nextStep []UserMessage // Steer / Inject
}
```

```
本包 agent/                     司机包 agent-loop/
  Roster 登记处   真代码           Driver            填工厂、造 Chat
  Inbox  结构体   真代码           Chat 实现         Followup 函数体、转圈
  Chat   接口     只有签名 ──────► 认这个接口
  Ticket 接口     只有签名
```

换司机也还用同一个 Inbox。

---

## 4. 事件（契约在本包，所以不 import 司机）

名字 → 回调列表。司机和别人都 `Subscribe` 这里写出的名字。

```
Broadcast（通知，调完返回，不拦）
  agent/created   disposed   status
  agent/session-start
  agent/inbox/inserted  claimed  discarded
  agent/error

RunChain（调 next 放行，不调即否决）
  agent/pre-step        这步进不进、进哪些消息
  agent/request         换成什么模型配置
  agent/request-error   失败了谁认领重试

按序等完（没有 next）
  agent/turn-stopping   轮次要关了；想续就 Steer
```

`turn/*` `step/*` `assistant/chunk` 是 session **日志**，不是 `agent/*`。UI 画对话读日志。

---

## 源码

```
src/
├─ index.ts              Provide("agents")
│                          live / factory / Create → Ticket
├─ runtime-types.ts      Chat 契约（源码名 Agent）+ agent/* 事件
└─ inbox.ts              Inbox 结构体
```

其余文件先不看。

---

## 记住

- 表上只有 `agents`。`live` 里是 Chat。Create 返回 Ticket。
- 定义者包可以有真代码（登记处、Inbox）。Followup 函数体在司机包。
- 入口：`Get("agents").Create` → `Followup`。换司机不换入口。
- `agent/*` 事件格式写在本包；谁当司机都认这套名字。
