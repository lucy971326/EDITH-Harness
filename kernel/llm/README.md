# llm

【它是什么】模型流式调用插件。

【提供能力】注册整份服务 `llm`，提供 `Client.Models` 和 `Client.Stream`。

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

`Models()` 只列出本机已配置 Provider 支持的模型及思考档位，供 Chat 首次选择；Chat 不读取或硬编码 `models.json`。模型与思考档位仍由本次 `RunConfig` 决定，不保存在 Client 中。
