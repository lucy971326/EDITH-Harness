# Session：对话账

persist 已过。这一步只写账本语义。不写 Runner、llm、按模型裁切图、cmd、sqlite、read_image。

---

## 心智模型

persist 是磁盘（节点：id / parent / 一包 JSON）。Session 才认识这句话是谁说的、光标在哪、沿哪条叉问模型。

一本聊天 = 一个文件 = 一棵树。分叉是树上的路，不是多本 session。

```
Host
  sessionPersistence  磁盘      persist.Add / Load     整棵树
  setups              人设旁    For / Put              不是账
  sessions            Store     Get / Create
                         └─ Session
                              Append / History / Branch
                              └─ persist.Add 写穿一行
```

不是喇叭。屏幕不听它。树就这一份：内存节点和磁盘同一串行。`History()` 现算，不另养一列消息。

```
【它是什么】     对话账。整份 A
【提供能力】     sessions → *Store；Get 得到 *Session
【使用能力】     sessionPersistence
【填充插槽】     不填
```

块 / Message 在 session。llm 以后来 import，不反过来。

---

## 光标、分叉、回退

`head` 是光标：现在站在哪一句。

```
Append  在 head 下面长一个子节点，立刻 persist.Add，head 跟着走
Branch  只挪光标。历史一行不改、不写盘
History 从 head 沿 parent 走回根，倒过来变成 []Message
```

磁盘整棵 Load 上来（分叉要留着）。`History` 不是少读文件，是从脚下往回爬，没踩到的不进。

```
n1 → n2 → n3 → n4
              ├─ n5  ← head     History = n1…n5
              └─ n6             在树上，不进这次
```

回退 = `Branch` 到旧节点。再说话从那里长新叉，旧路还在。工作区文件不还原——账本不管磁盘时光机。

重启后 head = 文件最后一行（最后一次 Append）。Branch 后还没说话就崩：回到崩前最后那句。指针不另存。

---

## 人设不是系统提示词

Setup 字段叫 `Persona`（人设），不叫 System。发给 SDK 的只有一段 system，先拼再传：

```
prompts 插件填的段      怎么用工具、怎么写代码     全家共用
Setup.Persona           你是后端助手               这一本的人设
        ↓
prompts.Assemble(setup) 现取 Persona，拼成纯文本
        ↓
llm.Request.System  →  SDK
```

人设在 `.setup.json`，不进账本。换人设 = `setups.Put`，下一轮再拼。本步不写 Assemble。

---

## 两种图

```
粘贴 / 拖进来     进账本。Media = MIME + Data（base64）
工作区文件路径    当文字。以后 read_image 去读。不进 Media
```

不落盘到 `media/`，不挂 attachments。前端以后压缩；后端以后卡上限。本步不写压缩。

发给模型时按这一轮能不能看图裁切，**不改账本**：

```
History()            账本原样（base64 还在）
    ↓ 裁切           有视觉：原样给 SDK
                     没视觉：换成「当前模型不支持看图」
    ↓
llm.Stream
```

`History()` 不裁切。裁切在 llm。本步不管。

人说话也是块。v1 认 `text` / `image`。`Kind` 是字符串。`image` 必须有非空 MIME 和 Data。

---

## 代码主干

```
kernel/session/
  plugin.go      Resolve("sessionPersistence")，挂 "sessions"
  types.go       Role / Block / Media / Message / UserMessage
  store.go       Store：Create / Get / List
  session.go     Append / History / Branch
  session_test.go
```

```go
type Role string // user | assistant | tool | system

type Media struct {
    MIME string // image/png
    Data string // base64。粘贴进来的图
}

type Block struct {
    Kind   string // text | reasoning | tool-call | image
    Text   string
    Tool   *ToolCall
    Media  *Media // 只有 image（以后 video）才有
    Error  string // 流出错入账；空 = 正常
}

type Message struct {
    Role   Role
    Blocks []Block
}

type UserMessage struct {
    Blocks []Block // 纯文字 = 一块 Kind=text
}

type Store struct {
    persist persist.Persistence
    live    map[string]*Session
}

func (s *Store) Create(id string) (*Session, error)
func (s *Store) Get(id string) (*Session, error) // live 或 Load。没有 → error
func (s *Store) List() ([]persist.Meta, error)

type Session struct {
    id    string
    disk  persist.Persistence
    nodes []persist.Node
    head  string
}

func (s *Session) Append(m Message) error // 写穿。parent = head
func (s *Session) History() []Message     // 现算。不按模型裁切
func (s *Session) Branch(id string) error
func (s *Session) Head() string
```

`Append`：编新 id → Body=Message 的 JSON → `persist.Add` → 成功才进 `nodes`、改 `head`。失败则内存不动。`Kind=image` 时 MIME、Data 都非空。

`Get` 同一 id 返回同一份（live），Branch 才看得到。

不写摘要压缩。`History` 沿当前叉整段走完。

---

## 测过再停下

1. Create → Append 用户句 → History 一句
2. Append 后新开 Store 再 Get：还在（写穿）
3. 分叉：在 n2 Branch 再 Append，两子都在树上；History 只沿当前 head
4. Branch 不删旧路（回退后再说话，旧叉还在）
5. Get 没有的 id → error
6. Install 后 Resolve("sessions") 拿到 Store
7. Append 带 image（MIME + Data）→ Load 后 jsonl 里有这段 base64
8. Append 缺 MIME 或 Data 的 image → error

过了再写 llm（空登记处 + Chunk + 按模型裁切图）。不要跳去 Runner。

---

## 账本长什么样

一本聊天两个文件。账本只在 jsonl 里。

```
{dir}/
  chat1.jsonl          账本。一行一个节点，只追加
  chat1.setup.json     Persona / 模型 / 工具名单。不是账
```

```json
{
  "kind": "llm",
  "persona": "你是后端助手",
  "model": "deepseek-v4",
  "reasoningEffort": "low",
  "tools": ["bash", "read"],
  "workspace": "/repo"
}
```

发生过的事：

```
n1  用户   「帮我改登录」+ 一张截图
n2  模型   思考 → 正文 → 要读 Auth.go
n3  工具   read 的结果
n4  模型   「用方案 A」
      ├─ n5 用户 「就用 A」     ← head
      └─ n6 用户 「改用 B」        旧路还在
```

磁盘 6 行，顺序是 Append 顺序。每行：`id` / `parent` / `body`（body = Message）。真正文件里每条占一行，下面拆开写。

**n1** 粘贴的图：base64 在账上。

```json
{
  "id": "n1",
  "parent": "",
  "body": {
    "role": "user",
    "blocks": [
      { "kind": "text",  "text": "帮我改登录，截图在这" },
      { "kind": "image", "media": { "mime": "image/png", "data": "iVBORw0KGgo..." } }
    ]
  }
}
```

**n2** 思考、正文、工具调用是同一条里的三块。蹦字不在账上。

```json
{
  "id": "n2",
  "parent": "n1",
  "body": {
    "role": "assistant",
    "blocks": [
      { "kind": "reasoning", "text": "先看现在的登录实现" },
      { "kind": "text",      "text": "我先读 Auth.go" },
      { "kind": "tool-call", "tool": { "id": "c1", "name": "read", "args": "{\"path\":\"Auth.go\"}" } }
    ]
  }
}
```

**n3** 工具结果。下一轮模型靠这一行才知道读过什么。

```json
{
  "id": "n3",
  "parent": "n2",
  "body": {
    "role": "tool",
    "blocks": [
      {
        "kind": "tool-result",
        "result": {
          "id": "c1",
          "name": "read",
          "content": "package auth\nfunc Login() {...}"
        }
      }
    ]
  }
}
```

**n4**

```json
{
  "id": "n4",
  "parent": "n3",
  "body": {
    "role": "assistant",
    "blocks": [
      { "kind": "text", "text": "建议用方案 A：把校验抽到 middleware。" }
    ]
  }
}
```

**n5 / n6** 两个儿子，同一个爹。

```json
{ "id": "n5", "parent": "n4", "body": { "role": "user", "blocks": [{ "kind": "text", "text": "就用 A" }] } }
{ "id": "n6", "parent": "n4", "body": { "role": "user", "blocks": [{ "kind": "text", "text": "改用 B" }] } }
```

head = `n5`。`History()` = n1→n2→n3→n4→n5。n6 不进。`Branch("n6")` 只改光标，文件一行不改。

发给有视觉的模型：n1 的 data 原样给 SDK。发给没视觉的：这一次请求换成字，**jsonl 里的 data 不动**。

「看 /repo/shot.png」是普通文字。以后 `read_image` 去读工作区，不写进 Media。

不进账本的：

```
Setup / Persona    旁边 .setup.json
蹦字 Delta         只给屏幕
todo / 棋盘        插件自己的文件
工作区图片文件     走工具，不走 Media
```

流出错：完整 Message `Append`，出错块带 `"error": "..."`。点停止：半截字不 Append。
