# llm

【它是什么】模型流式调用插件。

【提供能力】注册整份服务 `llm`，提供 `Client.Stream`。

【使用能力】不 Resolve 其他 Host 服务。

【填充插槽】不填。

## 代码主干

```text
Start
  → 读取 ~/.harness/config.yaml
  → 加载内置 models.json
  → RegisterService("llm", client)

Stream
  → 校验模型与思考档位
  → 转换历史和 Tool Schema
  → 调 goai
  → 返回流事件
```

模型与思考档位由本次 `RunConfig` 决定，不保存在 Client 中。
