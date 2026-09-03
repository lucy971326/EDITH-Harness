# persist

【它是什么】JSONL 持久化插件。

【使用能力】不 Resolve 其他服务。

【提供能力】同一份 JSONL 活对象注册三项服务：

- `sessionPersistence`：账本与元数据文件；
- `sessionSettings`：每场会话的模型、Agent、思考档位与工作区；
- `agentStore`：Agent 设置。

【填充插槽】不填。

【谁在用】`session` 用 `sessionPersistence`；`runner` 用 `sessionSettings`；`agents` 用 `agentStore`。

【不做】只管磁盘格式，不认识 Run、模型、页面或业务状态。
