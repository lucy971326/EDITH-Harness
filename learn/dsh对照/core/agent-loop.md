# 司机（源码包名 `agent-loop/`）

司机是造对话的那个人（进程一份）。Chat 是被造出来的那场对话（一场一份），说话和转圈都是它自己的事。

本包 = 司机 + Chat 的实现。契约、Inbox、`agent/*` 事件在登记处包。

本包还是消费者：`Get("sessions")` / `Get("llm")` / `Get("tools")` / `Get("systemPrompt")`。

源码把司机、Chat 实现、转圈都叫 loop。对照即可，讲解不跟着叫。

| 我们的词 | 源码 |
|---|---|
| 司机 Driver | 类 `AgentLoop`，键 `agentLoop` |
| 对话 Chat | 类 `ReactLoopAgent`，认接口 `Agent` |
| 转圈 | `agent.ts` 里的方法，不是第三个类型 |

Cordis 里司机还 `Provide("agentLoop")`（配置自造对话用）。入口仍是 `Get("agents").Create`。Go 里司机不必再占这个键。

---

## 登记处两格

```
登记处 agents
  factory     → 司机        只能一份
  live["s1"]  → 这场 Chat
  live["s2"]  → 另一场 Chat
```

```
启动     登记处.RegisterFactory(司机)
Create   司机 new Session + Chat → 放进 live → 返回票
Followup 票.Chat.Followup → inbox + wake → 这台对话转圈
```

票不是 Chat 的字段。说话走 Chat，拆场走票。`Get(id)` 只能说话。

---

## 1. 司机：填槽、造对话、发票

```go
type Driver struct{ host *Host }

func (d *Driver) Start(host *Host) error {
    // 源码还有：RegisterService("agentLoop", d)
    return host.Resolve("agents").(*Roster).RegisterFactory(d)
}

func (d *Driver) Create(opts CreateOpts) (*Ticket, error) {
    sess := sessions.Create(opts.SessionID)
    chat := NewChat(d.host, sess, opts) // 源码 ReactLoopAgent
    // 放进 live
    return &Ticket{Chat: chat}, nil
}
```

配置里写死要自造的，也走这条 `Create`，票留在司机手里，不对外发。

---

## 2. Chat 是结构体，活 = 还在 `live` 里

契约是接口。实现是结构体。字段三类：

```go
type Chat struct {
    // 1. 身份
    id string // = session.id

    // 2. 绑在身上的
    session Session
    inbox   *Inbox

    // 3. 转圈用的
    idle bool
    ctx  Context // 这场对话自己的小世界
}

func (c *Chat) Followup(msg UserMessage) {
    c.inbox.nextTurn = append(c.inbox.nextTurn, msg)
    c.wake()
}
```

Steer / Inject 同样进 inbox，列不同、是否 `wake` 不同。

「活」：堆上有这份实例，挂在 `live[id]`，字段会变，`wake` 后自己 `go kick()`。`Dispose` 从 `live` 拿掉，就不活了。

---

## 3. 转圈（Chat 的方法，不是第三个东西）

一轮 = 若干步。一步 = 一次模型请求 + 它点名的工具。先写日志，再问模型。

```
wake → kick: while turn()

turn
  Append turn/start
  循环 {
      Claim inbox
      RunChain("agent/pre-step")     不调 next = 这轮不要了
      Append step/start, user/message
      step: RunChain("agent/request") → llm.Stream → Append chunk/message
            有 tool-call → tools.Execute → 可能再进下一步
      Append step/end
      没活了 → 按序等完 "agent/turn-stopping"
  }
  Append turn/end
```

```go
func (c *Chat) wake() {
    if !c.idle {
        return
    }
    c.idle = false
    go c.kick()
}

func (c *Chat) kick() {
    for c.turn() {
    }
    c.idle = true
}
```

---

## 4. 事件：本包按登记处写出的名字喊

```
Broadcast     created / status / inbox/*
RunChain      pre-step / request / request-error
按序等完      turn-stopping
```

`turn/*` `step/*` `assistant/chunk` 是 session 日志，不是 `agent/*`。

---

## 源码

```
src/
├─ index.ts     司机：填工厂 + Create → 票
├─ agent.ts     Chat：Followup / 转圈
└─ tool-calls.ts  一步里的工具（后看）
```

---

## 记住

- `factory` = 司机，`live[id]` = Chat。
- Chat 是结构体：身份 + session/inbox + 转圈状态。活 = 还在 `live` 里。
- 转圈是 Chat 的私事。换司机：另写一份认工厂接口的实现，再填槽。
