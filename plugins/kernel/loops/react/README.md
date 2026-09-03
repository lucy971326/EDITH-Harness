# react

【它是什么】默认 ReAct 运行范式插件。

【使用能力】

- `llm`：流式调用模型；
- `tools`：取得本轮 Tool Schema，并执行模型请求的 Tool；
- `loops`：登记自身。

【提供能力】不注册整份服务；提供可运行的 `react` Loop。

【填充插槽】向 `loops` 登记 `react` Kind。

【谁在用】`runner` 根据 Agent 的 Kind 取出并执行它。

【不做】不读 SessionSettings、不写账本、不直接向浏览器发事件。
