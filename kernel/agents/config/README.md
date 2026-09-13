# agents/config

> Agent 设置的数据契约，不是 Agent 的运行逻辑。

- `Agent`：ID、名称、Kind、系统提示词和普通工具名单。
- `Store`：读取、保存、删除 Agent 设置。

真正的准备流程在上级 `kernel/agents`；文件存储由 `kernel/persist` 提供。
