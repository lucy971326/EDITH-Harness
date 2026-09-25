# internal

这里是 Go 后台：入口负责接线，领域模块负责各自的业务和状态。理解它们的配合，先跟一条用户消息走。

## 一条消息的主线

```text
启动  cmd/harness 或 cmd/harness-desktop
        → backend.Open：锁定数据目录、创建领域服务、登记实现，最后接入 appserver

请求  WebSocket 或 Wails Stream → appserver → conversations.Send
                                                  ├─ 空闲：runner.Start
                                                  └─ 运行中：runner.Steer

执行  runner → session/settings + agents.Prepare → loops/react
                                                     ├─ llm：生成文字和工具调用
                                                     └─ tools.Registry：校验、Hook、分发工具

结果  Loop.Emit → runner → session 落账 / events 发布 → appserver 通知 Client
```

1. [`backend`](backend/README.md) 在两个进程入口之间复用同一套组装。它把具体实现登记进工具、Loop、Skill 和命令目录；初始化失败或退出时逆序关闭资源。它不拥有会话业务状态。
2. [`appserver`](appserver/README.md) 处理连接、JSON-RPC 校验和方法分发。会话请求交给 [`conversations`](conversations/README.md)；模型、文件等公共请求交给各自的领域服务。空闲发送启动 Run 后返回；插话等 Runner 的检查点接纳。运行过程通过订阅送回 Client。
3. `conversations` 决定启动新一轮还是向当前轮插话。[`runner`](runner/README.md) 独占这一轮的运行、取消、草稿和收尾；它从 [`session/settings`](session/settings/README.md) 读取本轮设置，让 [`agents`](agents/README.md) 准备提示词、工具与执行类型，再选取 [`loops`](loops/README.md) 中的程序。
4. 当前程序是 [`loops/react`](loops/react/README.md)：反复调用 [`llm`](llm/README.md)，按模型请求调用 [`tools`](tools/README.md)，并在检查点接收插话。工具登记处先校验参数、运行 [`hooks`](hooks/README.md)，再交给具体工具；需要授权的操作由 [`permissions`](permissions/README.md) 计算规则、[`approvals`](approvals/README.md) 取得本次决定，实际本机操作由 [`machine/local`](machine/local/README.md) 执行。
5. Loop 将输出交回 Runner。流式片段先作为草稿事件发布；完整消息写入 [`session`](session/README.md) 对话账本后，再通过 [`events`](events/README.md) 发布耐久变化。appserver 把变化送到订阅连接；[`persist`](persist/README.md) 只负责文件读写，格式与恢复仍归各领域。

## 旁支如何接入

- [`subagents`](subagents/README.md)：工具触发子任务时创建父子关系与独立会话；子任务仍使用 Runner 执行，回报沿协作检查点交回父轮。
- [`skills`](skills/README.md)：给 Agent 准备可用技能；[`commands`](commands/README.md)：处理用户发起的命令，例如压缩。两者由 backend 登记具体实现。
- [`machine`](machine/README.md)：定义本机操作契约，`machine/local` 实现进程、文件与沙箱；`tools/exec`、`tools/applypatch` 等工具在其上组织模型可调用的操作。

读源码可沿 `backend/open.go → appserver/harness.go → conversations/service.go → runner/run.go → loops/react/react.go → tools/registry.go`。跨模块规则见[设计书](../docs/设计书.md)，数据归属见[DATA_MODEL](../DATA_MODEL.md)。

## 方法表（持续维护）

以下是 Client → appserver 的 JSON-RPC 请求方法，以注册代码为准；增删或改名时同步本表。`initialize` 和 `server/unsubscribe` 由连接层处理，其余由 `appserver` 登记。订阅返回 `subscriptionID`，用 `server/unsubscribe` 解除。

### 连接、会话与运行

- `initialize` → 握手，开始使用这条连接。
- `server/unsubscribe` → 解除本连接上的一项订阅。
- `workspace/select` → 打开原生目录选择框。
- `harness/session/create` → 为工作区创建或复用空会话。
- `harness/session/list` → 列出会话。
- `harness/session/get` → 读取会话信息与设置。
- `harness/session/settings/update` → 修改下一轮的 Agent、模型和权限等设置。
- `harness/session/fork` → 从已结束回答处分叉会话。
- `harness/session/send` → 启动一轮，或向当前轮插话。
- `harness/session/snapshot` → 读取账本与当前运行快照。
- `harness/session/subscribe` → 取得快照并订阅运行变化。
- `harness/session/stop` → 停止当前会话及其子任务。
- `harness/run/diff/read` → 读取某轮的单文件 Diff。
- `harness/run/diff/revertFile` → 按版本撤销某轮的文件修改。

### 子任务

- `harness/subagent/list` → 列出父会话的子任务。
- `harness/subagent/subscribe` → 取得子任务快照并订阅运行变化。
- `harness/subagent/send` → 给子任务发消息或插话。
- `harness/subagent/settings/update` → 修改子任务下一轮的模型与思考档位。
- `harness/subagent/stop` → 停止子任务。
- `harness/subagent/run/diff/read` → 读取子任务的单文件 Diff。
- `harness/subagent/run/diff/revertFile` → 按版本撤销子任务的文件修改。

### 可选能力与设置

- `model/list` → 列出可选模型。
- `agent/list` → 列出 Agent、执行类型和普通工具选项。
- `agent/save` → 新建或修改 Agent。
- `agent/delete` → 删除自建 Agent。
- `skill/list` → 列出会话工作区可用的 Skill。
- `command/list` → 列出平台命令。
- `command/call` → 在会话中执行平台命令。
- `permissions/modes` → 列出权限模式及可用状态。
- `approval/subscribe` → 取得待审批快照并订阅变化。
- `approval/respond` → 回答一次审批。
- `approval/settings/read` → 读取智能审批设置。
- `approval/settings/update` → 保存智能审批设置。
- `hooks/read` → 读取全局及项目 Hook 配置与信任状态。
- `hooks/save` → 保存 Hook 配置。
- `hooks/trust` → 确认项目 Hook 配置版本。

### 文件与终端

- `fs/readFile` → 读取文件内容及版本哈希。
- `fs/writeFile` → 版本未变时保存文件。
- `fs/readDirectory` → 列出目录的直接子项。
- `fs/searchPaths` → 在工作区搜索路径。
- `fs/getMetadata` → 读取文件或目录的类型与时间。
- `fs/watch` → 监听路径变化。
- `command/exec` → 启动交互终端，退出后返回状态。
- `command/exec/write` → 写入终端或关闭输入。
- `command/exec/resize` → 调整终端尺寸。
- `command/exec/terminate` → 终止终端进程。

服务端 → Client 的通知方法是 `harness/run/event`（运行变化）、`approval/changed`（待审批变化）、`fs/changed`（文件变化）和 `command/exec/outputDelta`（终端输出）；它们不能由 Client 当请求调用。
