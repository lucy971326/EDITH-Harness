# persist

【它是什么】JSONL 持久化插件。

【使用能力】不 Resolve 其他服务。

【提供能力】同一份 JSONL 活对象注册三项服务：

- `sessionPersistence`：账本、元数据，以及运行记录文件的原始字节；
- `sessionSettings`：每场会话的模型、Agent、思考档位与工作区；
- `agentStore`：Agent 设置。

【填充插槽】不填。

【谁在用】`session` 用账本与元数据；`runner` 用会话设置和运行记录字节；`agents` 用 `agentStore`。

【不做】只管磁盘格式，不解释运行记录内容，不认识模型、页面或业务状态。
