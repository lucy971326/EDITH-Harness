# session/（`packages/core/session/`）

B 登记处。ctx 键只有 `sessions`。

`Session` 是活对象：只追加的对话日志。契约和实现都在本包。没有工厂插槽。`Create` 就是 `new Session`，放进 `live`。

本包不写磁盘。磁盘在 `packages/session/`（`sessionPersistence`），听 `session/event` / `session/flush`。

---

## 这包认什么

定义者是本包：自己 `Provide("sessions")`，并写出 `Session`（`Append` / `DeriveMessages`）、`session/*` 事件、日志条目形状。

| | 谁 | 干什么 |
|---|---|---|
| 定义者 + 实现 | 本包 | 空登记处 + 活 `Session` |
| 消费者 | Chat 转圈时 | `Append`、问模型前 `DeriveMessages` |
| 消费者 | UI / 持久化 | 听 `session/event`；flush 时落盘 |

不跑 turn、不调 llm、不 `Provide` 磁盘后端。

对照 `agents`：两边都是登记处 + `live`。那边 Chat 的函数体在司机包；这边 `Session` 函数体就在这。

```
Get("sessions")
     └─ live["s1"] → Session
            Append("turn/start", …)
            DeriveMessages()
```

司机造对话：先 `Session`，再绑 Chat。`id` 相同。

---

## 1. 登记处

```go
type Sessions struct {
    live map[string]*Session
}

func SessionsPlugin(host *Host) error {
    host.RegisterService("sessions", &Sessions{live: map[string]*Session{}})
    return nil
}

func (s *Sessions) Create(id string) *Session {
    sess := NewSession(id)
    s.live[id] = sess
    Broadcast("session/created", sess)
    return sess
}

func (s *Sessions) Get(id string) *Session { return s.live[id] }
```

司机用 Enter / Announce，让 `Session` 和 Chat 一起发布。普通调用方用 `Create`。

---

## 2. 活对象

```go
type Session struct {
    id  string
    log []Event // 只追加
}

func (s *Session) Append(typ string, data any) Event {
    ev := Event{Type: typ, Seq: len(s.log), Data: data}
    s.log = append(s.log, ev)
    Broadcast("session/event", s, ev) // 已经写入；回调不否决
    return ev
}

func (s *Session) DeriveMessages() []Message {
    // 从日志折模型历史。chunk / turn 边界不是一条消息
    return nil
}
```

先 `Append`，再问模型。模型可见即已记录。问模型的历史来自这份日志，没有另一份数组。

```
turn/start
  step/start
    user/message
    assistant/chunk …
    assistant/message
    tool/call
    tool/result
  step/end
turn/end
```

`todo/write`、`request/header`、inbox 变更也是日志条目，先不用记全。fork / repair 旁支。

---

## 3. 实时事件 ≠ 日志条目

```
Broadcast              session/created  disposed
                       session/event    刚 Append 完
等全部听完（不否决）    session/flush    持久化把缓冲写出
```

| | 干什么 |
|---|---|
| `session/event` | 实时：日志又多了一条 |
| `turn/start` | 日志里的一条事实 |
| `tools/result` | `tools/` 流水线刚结束 |
| `tool/result` | 日志里的一条，模型下次看得到 |

UI 听 `session/event`，或读 `session` 已有事件。

---

## 源码

```
src/
├─ index.ts     Provide("sessions")；Create / Get；class Session
├─ types.ts     日志条目形状
└─ surface.ts   DeriveMessages（后看）
```

---

## 记住

- `sessions.live[id]` → `Session`。无工厂。实现在本包。
- Chat `Append`；问模型用 `DeriveMessages`。
- 本包内存。`packages/session/` 才是磁盘。
