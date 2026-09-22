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

它与消息账本分开保存。这里定义数据和 Store 接口；实际文件读写由 `kernel/persist` 提供。

权限模式默认 `ask_for_approval`；旧文件缺字段时读取补默认值，未知值报错。一次批准的额外权限不保存在这里。模式保存不等于执行层已经实施沙箱限制，接入状态见根目录 `STATUS.md`。
