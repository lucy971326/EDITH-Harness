# llm/（`packages/llm/llm/`）

B 登记处。ctx 键只有 `llm`。包不在 `packages/core/`，但是主干。

本包 **不打 HTTP**。适配器来 `registerAdapter`。对话转圈时来 `stream`。

`LlmAdapter` 不是活对象。它是插槽里的一条：若干 provider 路由 + `stream`。对照 `Tool`。

---

## 这组认什么

| 包 | 身份 | ctx 键 |
|---|---|---|
| `llm/` | 定义者：空登记处 + 词汇 | `llm` |
| `llm-deepseek/` | 填充者。路由 `deepseek-official` | 无新键 |
| `llm-pi-ai/` | 填充者。一份插件多条路由（openai / anthropic / …） | 无新键 |
| `llm-retry/` | 听 `agent/request-error`，**不**包 `stream` | 无 |
| `token-meter/` | 另 Provide | `tokenMeter` |

后两个点名即可。

| | 谁 | 干什么 |
|---|---|---|
| 定义者 | 本包 | `Provide("llm")`；写出 `LlmAdapter` / `GenerateOptions` / `StreamChunk` / `ToolSchema` |
| 填充者 | deepseek / pi-ai | `registerAdapter(路由们, 适配器)` |
| 消费者 | Chat 每一步 | `Get("llm").stream`。compaction 标题也会调，旁支 |

`options.provider` 选适配器；`options.model` 给适配器自己解析。路由没人填 → `NO_ADAPTER`。

```
Get("llm")
     adapters["deepseek-official"] → DeepSeekAdapter
     adapters["openai"]            → PiAiAdapter
     stream(opts)                  ← Chat 调登记处，不自己翻 map
```

---

## 1. 登记处

```go
type LlmAdapter interface {
    Stream(opts GenerateOptions) <-chan StreamChunk
}

type LLM struct {
    adapters map[string]LlmAdapter // key = provider 路由
}

func LlmPlugin(host *Host) error {
    return host.RegisterService("llm", &LLM{adapters: map[string]LlmAdapter{}})
}

func (l *LLM) RegisterAdapter(providers []string, a LlmAdapter) {
    for _, p := range providers {
        l.adapters[p] = a
    }
}

func (l *LLM) Stream(opts GenerateOptions) <-chan StreamChunk {
    a, ok := l.adapters[opts.Provider]
    if !ok { /* finish{error: NO_ADAPTER} */ }
    // waterfall("llm/stream") 后再 a.Stream(opts)
    return a.Stream(opts)
}
```

`RegisterAdapter` = 填槽。`Stream` = 使用。别混。

一份适配器可占多条路由（pi-ai）。一条路由只能一份适配器。

---

## 2. 一次调用怎么走

Chat 一步里：

```
Assemble                → system 文本 + tools[]（schema，不是提示词正文）
DeriveMessages          → 历史
stream({
    provider, model, reasoningEffort,   // 档位可空，空则适配器填默认
    system, messages, tools,
})
for chunk {
    Session.Append("assistant/chunk")
    assembler.Push(chunk)
}
Session.Append("assistant/message")   ← assembler 拼好的块
```

分片：`block-start` / `text-delta` / `tool-call-delta` / `block-end` / `usage` / `finish`。
`BlockAssembler` 把分片收成块。适配器抛错会被登记处收成 `finish{error}`，不往外扔。

本包 **不重试**。失败结束这一次 `stream`。`llm-retry` 听司机的 `agent/request-error`，让 Chat 再开一步。

---

## 3. 请求里有什么

```go
type GenerateOptions struct {
    Provider         string
    Model            string
    ReasoningEffort  string // 不透明 id。可空。没有 Thinking 开关
    System           string
    Messages         []Message
    Tools            []ToolSchema // JSON 字段，不是拼进 System
}
```

`ToolSchema` 写在本包，因为请求长这样。`tools/`、`systemPrompt` 都从这儿 import。

档位：登记处只认这一串 id。名单和默认值是适配器 `resolveModel` 公布的；空就填 `defaultEffort`，不在名单里先拒绝。

DeepSeek 插件另有部署开关 `thinking: enabled|disabled`，**不进** `GenerateOptions`。适配器把档位翻成线上字段：

```
off            → thinking: disabled
low|high|max   → thinking: enabled + reasoning_effort
部署关掉 thinking 还选档 → 拒绝
```

词汇（`Message`、内容块）也在本包。session 日志、司机、适配器共用，本包不写日志。
