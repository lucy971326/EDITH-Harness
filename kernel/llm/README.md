# llm

【它是什么】模型流式调用插件。

【使用能力】不 Resolve Host 服务；启动时读取 `~/.harness/config.yaml` 与内置模型清单。

【提供能力】注册服务 `llm`：`Models()` 返回可选模型/思考档位，`Stream()` 执行一次模型流。

【填充插槽】不填。

【谁在用】`react` 用 `Stream()` 调模型；`chat` 用 `Models()` 画选择框。

【不做】不保存会话、不决定本轮配置、不直接写页面。
