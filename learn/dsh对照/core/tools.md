# tools/

`packages/core/tools/` = **B 登记处**。表上只有一个键 `tools`。

本包 **不实现 bash/读文件**。那些插件来 `Register`。对话转圈时来 `Execute`。

`Tool` 不是活对象。它是插槽里的一条：名字 + 怎么跑。

对照 `agents`：那边 `live` 里是一场场 `Agent`；这边登记的是一条条 `Tool` 定义。没有 `Create` 出来的活 `Tool`。

---

## 这包认什么

表上有一把键 `tools`，B 登记处。不跑 shell，不写 turn/step 日志。

四张纸都是本包写的（定义者）。别人只认、不改。

交货就两种。身份还是定义者 / 提供者 / 消费者：

| | A 整份（shell） | B 登记处（本包 tools） |
|---|---|---|
| 定义者 | 只出 `Shell` 接口，不装 | 本包：出登记处方法 + **一条长什么样（`Tool`）**，自己 `Provide` 空登记处 |
| 提供者 | `bash-local`：`Provide("shell", 整份)` | `tool-bash`：按 `Tool` 来 `Register` 一条。不另占 ctx 键。心得叫填充者，避免和 A 撞名 |
| 消费者 | `tool-bash`：`Get("shell")` | Chat：`Get("tools").Execute`，不自己翻 `items` |

`Tool` 就是定义者顺带写出的提供者接口。没有 `ctx.bash`，一条工具不是一个 ctx 服务。

```
Get("tools")                      ← 服务（B）。定义者自己 Provide
     │
     ├─ items["bash"]  → Tool     ← 槽的提供者 Register 进来
     ├─ items["read"]  → Tool
     │
     └─ Execute("bash", args)     ← 消费者调登记处方法，不自己翻 `items`
```

`tool-bash` 一身两役：对 `shell` 是 A 的消费者，对 `tools` 是 B 的提供者（填充者）。

本包启动时还会往 `systemPrompt` 接一条：「组装时把当前可见工具 schema 列出来」。不是司机送的。

---

## 1. 登记处

```go
type Tool struct {
    Name    string
    Execute func(args string, exec Exec) (string, error)
}

type Tools struct {
    items map[string]Tool
}

func ToolsPlugin(host *Host) error {
    host.RegisterService("tools", &Tools{items: map[string]Tool{}})
    return nil
}

func (t *Tools) Register(item Tool) {
    t.items[item.Name] = item
}

func (t *Tools) Execute(name, args string) (string, error) {
    tool, ok := t.items[name]
    if !ok {
        return "", errors.New("unknown tool")
    }
    // 先走事件门禁，再调这条的 Execute
    return tool.Execute(args, Exec{})
}
```

`Register` = 填槽。`Execute` = 使用。别混。

---

## 2. 一次调用怎么走

对话转圈发现模型点了工具，不自己 `items["bash"]`，只调登记处：

```
Get("tools").Execute(...)

  RunChain("tools/pre-execute")   放行 / 拒绝
  guard()                         单调拒绝，不能再改回允许
  RunChain("tools/execute")       包一层（超时、重试）；next = 真正跑 Tool.Execute
  RunChain("tools/post-execute")  改结果或拦住
  Broadcast("tools/result")       通知，不拦
```

`tools/*` 是实时事件。对话随后往 session 日志写的是 `tool/call` / `tool/result`。名字像，不是同一个东西。

---

## 3. 填槽长什么样

```go
func ToolBash(host *Host) error {
    sh := host.Resolve("shell").(Shell)
    tools := host.Resolve("tools").(*Tools)
    tools.Register(Tool{
        Name: "bash",
        Execute: func(cmd string, exec Exec) (string, error) {
            r := sh.Run(sh.Resolve(cmd))
            return r.Out, nil
        },
    })
    return nil
}
```

schema（给模型看的名字和参数）跟着 `Register` 那条走。组装提示词时本包再按当前名单列出来。

---

## 源码

```
src/
├─ index.ts     Provide("tools")；Register / Execute；tools/* 事件
├─ types.ts     往 session 日志合并的 code-dispatch 事件（后看）
└─ schema.ts    defineTool 辅助（后看）
```

code 模式、presentAs、restrict 先不看。

---

## 记住

- 表上只有 `tools`。`Tool` 是插槽条目，不是活对象。
- 填槽：`Register`。使用：`Execute`。司机不 import `tool-bash`。
- 门禁是 `tools/*` 回调；写进对话历史的是 session 的 `tool/*`。
