# llm

读取模型配置，通过 goai 发起流式模型调用。

```text
RunConfig + Input
  -> Client.Stream -> 消息 / 工具转换 -> goai -> 流事件
```

- `client.go`：构造、模型查询、Stream 主线。
- `config.go`：本机 `config.yaml`；`models.go / models.json`：内置模型能力与思考档位。
- `messages.go`：账本消息与工具转成 Provider 输入。
- `types.go`：调用配置与输入形状。

ReAct、压缩与智能审核复用此客户端。模型配置在启动时读取；这里不保存会话，也不决定本轮执行流程。
