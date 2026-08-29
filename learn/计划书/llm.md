# LLM：模型路由与协议适配

Session 已过。这一步钉模型怎么选、请求怎么适配。**状态永远在本地 Session；远端只处理这一轮完整历史。**

旧《插件设计书》的 llm「Host 登记处 B，Adapter 往里 Register」作废。本篇为准。

---

## 心智模型

```text
Setup.Model
    ↓
llm.Service（Host 的 "llm"，整份 A）
    ├─ Provider 配置：地址、密钥、默认 API、启用模型
    ├─ 本地 Catalog：模型事实、请求补丁
    └─ 编译进来的 Adapter：协议转换、HTTP/SSE
```

```text
【它是什么】     模型路由与统一流。整份 A
【提供能力】     "llm" → *Service；按模型 Stream
【使用能力】     不 Get 其他服务；调用者给本轮请求
【填充插槽】     不填。Adapter 不往 Host 注册
```

LLMAgent 后来只 `Resolve("llm")`。它不认识 Provider、API 或 Adapter。

---

## 状态只在本地

```text
Session.History()
    ↓
按当前模型能力复制、投影图片
    ↓
Adapter 转线上请求
    ↓
Stream
```

三种线上协议均无状态：

| API | 本轮发送 |
|---|---|
| `completions` | 完整 `messages[]` |
| `responses` | 完整 `input[]`；**不传 `previous_response_id`** |
| `messages` | 完整 `system + messages[]` |

不保存 response ID，不让服务端续聊。协议私货不写入账本；Adapter 每轮都从可见的本地历史重新翻译请求。

---

## Provider、Model、Adapter

### Provider 配置：部署事实

Provider 是本次实际连接的 endpoint、认证和默认 API。Models 嵌在 Provider 下：

```yaml
llm:
  providers:
    deepseek:
      catalog: deepseek
      api: completions
      baseURL: https://api.deepseek.com
      apiKeyEnv: DEEPSEEK_API_KEY
      models:
        deepseek-chat: {}
        deepseek-reasoner: {}

    gateway:
      catalog: openai
      api: completions
      baseURL: https://gateway.example.com/v1
      models:
        gpt-5:
          api: responses # 可覆盖 Provider 默认 API
```

模型的最终 API：

```text
Model.api > Provider.api
```

`Setup.Model` 是唯一选择入口：

```yaml
model: deepseek/deepseek-reasoner
```

它选中 Provider、模型、API、能力和远端模型 ID。换模型只改这一格；Setup 不加 Provider、Adapter、System。

Setup 是这本聊天配置的唯一事实来源。前端改模型或思考档位先 `setups.Put`，再发消息；Runner 每次 Run 都 `For` 一份快照。正在运行的一轮不受之后的 Put 影响，下一轮才读到新值。不支持仅一轮的临时覆盖。

### Catalog：模型与请求事实

models.dev 的 `api.json` 下载后处理成本地 Catalog；运行时不依赖网络。

它提供通用事实：

```text
vision     = modalities.input 有 image
context    = limit.context
tools      = tool_call
reasoning  = reasoning + reasoning_options
```

不要用 `attachment` 判断视觉；不要把 models.dev 的 `npm` 当 API。

models.dev 不知道一个思考档位在线上该写什么字段。因此生成 Catalog 时合并 Harness 自己维护的少量**请求补丁**：

```json
{
  "id": "deepseek-reasoner",
  "vision": false,
  "api": "completions",
  "reasoning": {
    "off":  { "thinking": { "type": "disabled" } },
    "high": {
      "thinking": { "type": "enabled" },
      "reasoning_effort": "high"
    }
  }
}
```

另一模型可有不同补丁：

```json
{
  "id": "claude-sonnet",
  "api": "messages",
  "reasoning": {
    "high": {
      "thinking": { "type": "adaptive" },
      "output_config": { "effort": "high" }
    }
  }
}
```

请求补丁是受版本控制的可信 Catalog 数据，**不是用户 YAML 的任意 JSON 注入**，也不另造 `dialect` 层。

`ReasoningEffort` 的规则：

```text
""       不显式指定，交给远端默认
非空      必须是当前模型 reasoning map 的键；取对应请求补丁
没有该键  发请求前 error
```

`off` / `on` / `high` 等只是模型 Catalog 给这台模型开放的名字；不再假设所有模型有同一组选项。

### Adapter：协议代码

三份 Adapter 编译进程序：

```text
completions
responses
messages
```

Adapter 只做：

```text
完整本地消息 → 该 API 的消息格式
构造该 API 的基础 body
补充 / 覆盖当前模型的 reasoning 顶层请求字段
SDK / HTTP 发起流
线上 SSE / SDK 事件 → 统一 Chunk
```

它不管理 Setup、模型选择、Catalog、Host、Session、工具执行或服务端会话状态。

新增 Provider：

```text
已有 API + 已有模型请求补丁  → 只改 Provider YAML
已有 API + 新思考形状         → 补 Catalog 数据，不改 Go
全新消息/流协议               → 新写 Adapter
```

SDK 只是 Adapter 内部细节。SDK 已有的字段优先填强类型 Params；兼容 Provider 的未知顶层字段用 SDK 的扩展机制（官方 Go SDK 为 `SetExtraFields(map[string]any)`）补充或覆盖。Catalog 的补丁给出完整顶层值，不做任意深合并；例如一次给完整的 `thinking` 对象。SDK 没有扩展机制时，Adapter 才自己构造该 API 的 JSON body；不能让 SDK 类型决定内核契约。

---

## 图片

`History()` 原样返回账本。Service 在发送前深拷贝并投影：

```text
Vision=true   保留 image 的 MIME + base64
Vision=false  原位置换 text：「当前模型不支持图片查看」
```

不回写账本，不读工作区路径，不把裁切结果存盘。

---

## 服务与流形状

```go
type Request struct {
    System   string
    Messages []session.Message
}

func (s *Service) Stream(
    ctx context.Context,
    setup kinds.Setup,
    req Request,
) (<-chan Chunk, error)
```

`System` 是 prompts 以后组装的本轮文本；不是 Setup 字段。模型和思考档位只从传入的 Setup 快照读取。

```go
type Chunk struct {
    Index  int
    Kind   string // reasoning | text | tool-call
    Delta  string
    Tool  *session.ToolCall
    Done  bool
    Usage  *Usage
    Err    error
}
```

- 同一 `Index` 的 `Delta` 拼接；工具参数 JSON 也走 `Delta`。
- `Tool` 只在 tool-call 第一片给 ID、Name。
- 更大的 Index 不代表较小块结束；不同块可以交错。
- `Done` 是正常终态，`Err` 是失败终态；两者互斥，终态后 channel 关闭。
- Runner 以后测时长；Usage 仅是模型返回的 token 数据。

---

## 本步代码与停点

```text
kernel/llm/
  plugin.go      构造并挂 "llm"
  types.go       Config / Catalog / Request / Chunk / Adapter
  service.go     provider+model 解析、API 路由、请求补丁选择
  project.go     图片投影
  llm_test.go
```

构造入口：

```go
func New(cfg Config, catalog Catalog, adapters map[string]Adapter) (*Service, error)
```

本步测试直接构造 Config、Catalog 和 fake Adapter；不写 yaml 解析、models.dev 下载器、真实 HTTP Adapter、tools、Runner、prompts、自动上下文裁切、重试或 fallback。

测过再停：

1. Install 后 `Resolve("llm")` 得到 `*Service`
2. `provider/model` 正确解析 Provider 默认 API，模型 API 可覆盖
3. 同一 Adapter 服务多个模型；远端模型 ID 随模型变
4. 未知 Provider / 模型 / API、未编译 Adapter → error
5. 非空思考档位只接受 Catalog 中的请求补丁
6. Vision 投影不改原 History；无 Vision 的请求不含 base64
7. Adapter 收到完整历史；Responses 请求没有 `previous_response_id`
8. 同 Index 的 Chunk 可交错；Done / Err 终态约束成立

过了才写第一个真实 `completions` Adapter。之后用同一契约补 `responses`、`messages`。
