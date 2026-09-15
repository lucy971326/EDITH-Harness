# Subagent 工作页面

## 目标

点击主聊天里的 Subagent 卡片，在辅助面板打开可交互标签页：

```text
主聊天点击 → 右侧 Subagent 标签
                ├─ 实时工作过程
                ├─ 继续对话 / Steer
                ├─ 停止
                ├─ 空闲时切换模型和推理档位
                └─ 查看 Diff
```

不自动弹出，不出现在辅助面板“＋”菜单。关闭标签只关闭视图，不停止任务。

## 后端

- `subagent_spawn` 增加必填 `taskName`；任务关系持久化稳定名称。旧记录继续读取，名称回退到 Agent 名称。
- Harness Product 增加受控的 Subagent 查询、订阅、发送、设置、停止和 Diff 操作；全部使用 `parentSessionID + taskID` 校验归属，不向 Web 开放任意子 `sessionID`。
- 增加 JSON-RPC 方法：
  - `harness/subagent/list`
  - `harness/subagent/subscribe`
  - `harness/subagent/send`
  - `harness/subagent/settings/update`
  - `harness/subagent/stop`
  - `harness/subagent/run/diff/read`
  - `harness/subagent/run/diff/revertFile`
- `send` 在子任务忙时 Steer，空闲时开启新一轮；主 Agent 已结束也能发送。每轮结果继续通过现有协作消息进入父账本。
- 复用现有 Runner 快照、事件订阅、Diff 和同一条 WebSocket；每个打开的标签只拥有独立 `subscriptionID`。
- Agent 类型和工作区保持创建时配置；仅允许子任务空闲时修改模型与推理档位。

## 前端

- 抽出共享 `ConversationSurface`：复用消息时间线、工作过程、Tool、图片、Diff、输入框、滚动与运行状态；主聊天和 Subagent 只提供不同的数据控制器。
- 主聊天识别 Subagent 创建及协作消息，使用专用 `Bot` 图标和状态标识；点击后传递 `parentSessionID + taskID`。
- `WorkspaceTabs` 登记 `subagent` 视图并管理去重标签；切换主会话时关闭其 Subagent 标签和订阅。
- `AuxiliaryView.onCreate` 改为可选；“＋”菜单只展示可主动创建的视图，因此 Subagent 只能由主聊天打开。
- `SubagentView` 负责自己的草稿、订阅和设置状态；隐藏分叉与普通会话导航，保留发送、停止、模型、推理档位和 Diff。
- 一个页面始终只有一条共享 WebSocket；关闭标签只取消对应订阅。

## 验证

- 后端：归属隔离、旧任务兼容、父会话空闲时续聊、运行中 Steer、单独停止、运行中禁止改设置、结果回传父账本、订阅顺序与 Diff 撤销。
- 前端：点击打开、重复点击激活原标签、多 Subagent 标签、菜单中无 Subagent、关闭不停止、断线重连重新订阅、窄面板布局。
- 完成后运行一次 `make agent-check`；由用户执行 `make run` 并截图验收，不由 Agent 操作浏览器。
