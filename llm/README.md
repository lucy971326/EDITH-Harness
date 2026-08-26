# llm

一句话：**模型插座排**，统一模型请求；具体服务商自己插进来。

```text
DeepSeek / 其他服务商插件
            │ Register(adapter)
            ▼
          llm
            │ Stream / Validate / Providers
            ▼
          loop / Web UI
```

- 提供能力：`"llm"`
- 本身没有任何模型；`llmplugins/deepseek` 才是一个具体填充物。
- 先读：`plugin.go` → `service.go` → `types.go`。
