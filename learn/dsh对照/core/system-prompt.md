# system-prompt/

B 登记处。ctx 键只有 `systemPrompt`。没有活对象。本包不写全文、不调 llm。

定义者是本包：`Provide` 空登记处，写出 `Section` / `Variable` / `Tools`（填槽）和 `Assemble`（使用）。

| | 谁 | 干什么 |
|---|---|---|
| 定义者 | 本包 | 空登记处；自己先填身份段 |
| 提供者 | 工具插件、persona、`tools/`、司机 | 填段 / 工具名单 / `{{model}}` `{{cwd}}` |
| 消费者 | Chat 每一步 | `Assemble`，送给 llm |

`tools/` 自己把当前可见 schema 接到这里，不是司机送的。每次步骤现拼，不另存一份。
`Assemble` 返回两样：段文本 → `system`；`ToolSchema[]` → llm 请求的 `tools` 字段。不是把 JSON 写进提示词正文。

```
Get("systemPrompt")
  sections[] / variables[] / tools()     ← Register
  Assemble()                             ← Chat 用
```

```go
type SystemPrompt struct {
    sections  map[string]Section
    variables map[string]func() string
    toolList  func() []ToolSchema
}

func (p *SystemPrompt) Section(s Section) { p.sections[s.Name] = s }
func (p *SystemPrompt) Variable(name string, fn func() string) { p.variables[name] = fn }
func (p *SystemPrompt) Tools(fn func() []ToolSchema) { p.toolList = fn }
func (p *SystemPrompt) Assemble() Assembly {
    // 段 "你是 {{model}}" + 变量 model→"v3" → "你是 v3"
    return Assembly{}
}
```

Chat 一步里：`Assemble` → `RunChain("system-prompt/assemble")` → 渲染 `{{var}}` → `llm.Stream`。历史仍来自 `Session.DeriveMessages`。

事件：`system-prompt/assemble`（改拼装）；`system-prompt/change`（有人填/卸）。源码就 `src/index.ts`。
