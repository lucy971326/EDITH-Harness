# persist

【它是什么】Harness 固定的本机文件持久化服务。数据根目录通常为 `~/.harness`，不提供 SQLite 或介质切换配置。

【使用能力】不 Resolve 其他服务。

【提供能力】注册一份 `persist` 文件服务，并在它上面提供三项领域存储契约：

- `persist`：限定根目录与子作用域，统一读取、列举、删除、同步追加和原子替换；

- `sessionPersistence`：账本、元数据，以及运行记录文件的原始字节；
- `sessionSettings`：每场会话的模型、Agent、思考档位与工作区；
- `agentStore`：Agent 设置。

【填充插槽】不填。

【谁在用】`session` 用账本与元数据；`runner` 用会话设置和运行记录字节；`agents` 用 `agentStore`；LLM、MCP、Skill 用户目录与内置 Skill、Subagents 使用各自的 `persist` 作用域。

【不做】不解释业务数据，不决定保存时机，不提供跨文件事务。每个模块负责自己的结构、校验与恢复规则。

固定格式：消息账本使用 JSONL；设置、元数据、运行记录与子任务使用 JSON；LLM 配置使用 YAML；Skill 使用 `SKILL.md`。格式按数据用途确定，不做全局统一后缀。
