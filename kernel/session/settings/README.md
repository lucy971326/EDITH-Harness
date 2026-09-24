# session/settings

> 一场会话“下一轮如何运行”的设置契约。

```text
SessionSettings
├─ AgentID
├─ Model
├─ ReasoningEffort
├─ Workspace
└─ PermissionMode
```

它与消息账本分开保存。这里拥有数据、Store 契约与 JSON 读写；`persist.Files` 只提供底层文件操作。

权限模式默认 `ask_for_approval`；新设置保存时补默认值；文件缺字段或含未知值直接报错，不迁移旧数据。一次批准的额外权限不保存在这里。模式保存不等于执行层已经实施沙箱限制，接入状态见根目录 `STATUS.md`。
