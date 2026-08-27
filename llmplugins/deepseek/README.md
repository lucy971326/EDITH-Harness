# llmplugins/deepseek

一句话：**DeepSeek 翻译官**，把统一 LLM 请求翻成 DeepSeek SDK 调用。

```text
loop / Web UI
      │ llm.Request
      ▼
     llm 插座
      ▼
 DeepSeek adapter
      ▼
 DeepSeek API
```

- 领取能力：`"llm"`、`"settings"`、`"credentials"`
- 填充插槽：`llm` 的 Adapter；settings 上名为 `deepseek` 的抽屉
- 负责模型目录、思考档位和流式响应翻译；不管会话流程。
- 先读：`plugin.go` → `adapter.go`。
