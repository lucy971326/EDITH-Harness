# internal

Go 后台按领域组织：入口接线，领域拥有业务规则与状态。先沿一条消息读主干：

```text
cmd/harness 或 cmd/harness-desktop
  → backend.Open：锁定数据目录、装配服务、登记实现、接入 appserver
  → WebSocket / Wails Stream → appserver → conversations.Send
  → runner.Start（空闲）或 runner.Steer（运行中）
  → agents.Prepare → loops/react → llm / tools.Registry
  → runner → session 落账、events 发布 → appserver 通知 Client
```

- [`backend`](backend/README.md) 复用两种进程入口的装配与逆序关闭，不持有会话业务状态。
- [`appserver`](appserver/README.md) 处理协议、连接和方法登记；会话操作交给 [`conversations`](conversations/README.md)，其他请求交给所属领域。
- [`runner`](runner/README.md) 拥有活 Run、取消、草稿与收尾；[`session`](session/README.md) 保存对话和元数据，[`session/settings`](session/settings/README.md) 保存每轮设置，[`reading`](reading/README.md) 保存已读位置。
- [`agents`](agents/README.md) 准备提示词与工具；[`loops/react`](loops/react/README.md) 调用 [`llm`](llm/README.md) 和 [`tools`](tools/README.md)。工具按需经过 [`hooks`](hooks/README.md)、[`permissions`](permissions/README.md)、[`approvals`](approvals/README.md) 与 [`machine/local`](machine/local/README.md)。
- [`subagents`](subagents/README.md) 保存父子关系并复用 Runner；[`skills`](skills/README.md) 与 [`commands`](commands/README.md) 由 backend 登记；[`events`](events/README.md) 发布变化，[`persist`](persist/README.md) 只负责可靠文件读写。

源码可沿 `backend/open.go → appserver/harness.go → conversations/service.go → runner/run.go → loops/react/react.go → tools/registry.go` 阅读。跨模块规则见[设计书](../docs/设计书.md)，数据归属见[DATA_MODEL](../docs/AGENTS/DATA_MODEL.md)。

## 对外方法从哪里查

JSON-RPC 方法以 [`appserver`](appserver/README.md) 各 `register`／`Bind` 入口的实际登记为准；手写 TypeScript 契约在 [`clients/contracts`](../clients/contracts/)，改方法名、参数或返回值时两边一起核对。`initialize` 与 `server/unsubscribe` 属于连接层。

会话方法集中在 `appserver/harness.go`：包括创建、列表、归档／恢复、删除、重命名、设置、定点分叉与末尾完整回答分叉、发送、订阅和停止。会话活动／已读、Run Diff 与子任务也从该文件登记；模型、MCP、Skill、审批、文件与终端等方法分别见 appserver 同名文件。服务端通知由各订阅和连接层发送，不能当作 Client 请求调用。
