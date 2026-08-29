# LLM 新计划

## 目的

LLM 只做一件事：把 Runner 给出的完整历史交给用户选中的模型，并返回远端流。

```text
Setup + 本轮输入
      ↓
模型定义 + 本机配置
      ↓
goai LanguageModel.DoStream
      ↓
goai StreamChunk
```

它不保存历史、不执行工具、不重试、不管理 Agent，也不自己解析 HTTP/SSE。

## 输入

```text
Setup.Model            选择 models.json 的模型名
Setup.ReasoningEffort  选择该模型 reasoning 中的一段 JSON；空则不传
Input.System           本轮系统提示词
Input.History          Session 的完整历史
Input.Tools            本轮工具 schema；工具真正执行仍归 Runner
```

## 两份数据

```text
kernel/llm/models.json
  随代码提交；没有密钥。
  写模型名、Provider、远端模型 ID、各思考档位的专有请求字段。

~/.harness/config.yaml
  本机部署数据；不进仓库。
  写 Provider 的 BaseURL 和 API key。
```

DeepSeek 的例子：

```json
"off":  { "thinking": { "type": "disabled" } }
"high": {
  "thinking": { "type": "enabled" },
  "reasoning_effort": "high"
}
```

`ProviderOptions` 会将这些专有字段交给 goai；所以新增一个已支持 Provider 的模型，通常只改 JSON。

## 代码边界

```text
Client.Stream
  1. 按 Setup.Model 找 models.json
  2. 按模型 Provider 找 YAML 配置
  3. 按 ReasoningEffort 取 JSON
  4. 构造 goai LanguageModel
  5. Session Message → goai Message
  6. 调 DoStream，原样返回 StreamChunk
```

goai 没有总构造器；`newModel` 里为实际启用的 Provider 写一个明确分支：

```go
deepseek.Chat(...)
anthropic.Chat(...)
google.Chat(...)
```

不建额外框架或注册表。当前只实现 DeepSeek；需要另一家时才加对应一行分支。

## 生命周期

```text
models.json    编译进程序，启动后不变
config.yaml    启动时读取一次
模型实例       每次 Stream 时创建，不缓存
```

当前 goai 要求 Go 1.25。

## 不做

```text
自写 Provider 协议、HTTP、SSE
自写统一流格式
模型下载或能力目录
自动工具循环
重试、fallback、热加载
```

工具结果历史的本地形状尚未定；遇到已有 tool role 历史时当前明确报错，而不是猜测转换。
