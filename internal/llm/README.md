# llm

```text
models.dev → build/models.mjs → catalog.json（随程序发布）
                                      + models.json（用户覆盖）
                                      ↓ Pin 固定本轮
历史 + 工具 → Client.Stream → OpenAI / Anthropic 官方 SDK → Harness 流事件
                              ↑ API Key / Codex、xAI OAuth
```

- `catalog.go / catalog.json`：生成的供应商、端点及模型能力；开发时执行 `make models-update`，构建和运行不联网更新。
- `models.go`：当前格式的目录合并、用户覆盖及有序思考档位；不兼容或迁移旧配置。
- `config.go / settings.go`：密钥与地址、设置读写和文件版本检查。
- `client.go / stream.go`：查询、固定本轮配置、调用分发及取消。
- `messages.go / openai.go / responses.go / anthropic.go`：消息转换、三种协议流解析；Codex 使用 Responses 的订阅请求形状。
- `codex/ / xai/`：账号登录与刷新；`oauth/`：私有凭据和设备码流程。授权不决定协议，SDK 类型不进入调用方。
- `types.go`：公共请求、流事件与设置契约。

供应商名单取 Pi 中 OpenAI／Anthropic 协议入口；不收录 Bedrock、Google／Vertex、Azure 专用适配、Mistral Conversations、Radius 和 TypeSafe。混合网关按模型选择协议，其他协议的模型过滤掉。保留现有 xAI 账号通道。

Cloudflare 在连接地址填写账号／网关 ID；Copilot 当前使用已换取的 API Token，不提供新的账号登录流程。

不同供应商复用协议，特殊思考参数由生成脚本的显式映射处理。模型目录表示已知能力，不表示账号获准调用；权限仍以供应商返回为准。签名／加密思考按供应商、模型和协议绑定，只在相同通道重放。

Responses 历史按原消息保留文字与图片顺序；流式接口同时接收增量和仅在最终事件给出的内容。工具 Schema 按通用协议原样发送。

ReAct、压缩与智能审核复用此客户端；这里不保存会话，也不决定本轮执行流程。

`context.go` 提供请求预算、含工具与媒体的保守估算，以及明确的上下文超限错误归类；不触发压缩、不读取账本。输出沿用模型配置，仍遵循各协议支持范围。
