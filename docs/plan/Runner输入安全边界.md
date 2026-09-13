# Runner 输入安全边界计划

## 目标

修复外部输入插入未完成工具批次的问题，并把 Runner 整理成一套统一、可维护的输入模型。

当前错误账本可能出现：

```text
助手 tool_call
子代理回报 / 用户 Steer
tool_result
```

目标顺序：

```text
一次模型步骤
├─ 助手输出
├─ 本批全部 tool_result
└─ 安全检查点
   ├─ 用户 Steer
   └─ 子代理回报
```

Session 账本仍是唯一事实来源。LLM、前端和 Subagents 不得私自重排或维护第二份历史。

## 核心设计

- 从一次 Checkpoint 返回到下一次 Checkpoint，视为一个不可插入的模型步骤；其中包含模型输出和该批全部工具结果。
- Steer 与协作回报统一进入 Runner 的“待提交输入”，立即触发 `InputSignal`，但暂不写账本。
- Checkpoint 按到达顺序将待提交输入写入 Session，成功后发布 Message 事件，再把同一批消息交给 Loop。
- Steer 只有在消息真正落账并发布后才返回成功；此前运行若结束或失败，则返回错误，消息不自动变成下一轮任务。
- `Runner.Receive` 对尚未落账的协作通知返回未确认；Subagents 保留通知。消息落账后，重试通过稳定 `MessageID` 确认并标记 delivered。
- Stop 保持独立控制路径：只取消 Run Context，不写账本、不进入待提交输入，也不参与消息排序。

## 实现调整

### Runner

- 将现有 `steers` 改造成统一的待提交输入集合，保存消息、到达顺序以及 Steer 的完成回执。
- 合并 Steer 与 collaboration 重复的准入、复制、信号、落账和发布流程；公开方法只保留各自的身份校验与返回语义。
- Checkpoint 在 `handoff` 边界内取得本批输入并完成顺序落账；发布事件时不持 Runner 锁。
- Checkpoint 有输入时用最后一条耐久输入更新 `outputAfterSeq`，并开启下一代 `InputSignal`；最终检查点无输入时原子关闭输入准入。
- Run 停止、失败或关闭时清理未落账输入，并唤醒所有等待中的 Steer 调用，避免请求或协程悬挂。
- 删除改造后不再需要的“外部输入自行发布并在收尾等待”状态，保留草稿、更新序号、工具定位与持久化去重职责。

### Product 与 appserver

- JSON-RPC 方法和数据格式不变。
- Product 不把等待 Steer 落账的时间扩大成跨会话的全局业务锁。
- appserver 保持 Stop 为控制调用；若 Steer 正等待 Checkpoint，同一连接仍必须能够处理 Stop，不能让传输层串行化削弱取消能力。

### Subagents

- `subagent_wait` 仍只返回等待结果，不携带协作正文。
- 完成通知通过现有投递流程进入 Runner；若当前模型步骤尚未结束，只登记为待提交，不提前标记 delivered。
- `subagent_wait` 的 tool result 必须先落账；若同批还有其他工具，等待全部 tool result；随后 Checkpoint 再写协作回报。
- 不增加 Subagent 专属事件、第二份对话历史或 `subagent_wait` 特判排序。

## 失败与恢复

- 待提交 Steer 只存在于当前活 Run 内存；由于尚未返回成功，崩溃或运行提前结束时允许丢弃，前端保留原草稿供用户决定是否重发。
- 协作通知的来源记录仍保存在 `subagents/tasks`；未得到父账本确认时保持未投递，后续活 Run 可重试。
- 已经被旧 Bug 写坏的历史不自动迁移、不静默重排；测试会话按需单独删除。
- 任一输入落账或事件发布失败都结束当前 Run，并把真实错误返回给等待方；不得谎报成功或自动重发。

## 验证

只补覆盖新风险的少量测试：

1. 子代理完成时，父账本严格为 `tool_call → 全部 tool_result → collaboration`，下一次模型请求可正常接受。
2. 工具执行期间收到 Steer，账本严格为 `tool_call → 全部 tool_result → user`，Checkpoint 与下一次模型请求看到相同顺序。
3. 最终检查点、停止和运行失败能拒绝未落账输入并解除等待；协作通知保持可重试。
4. Steer 等待落账时 Stop 仍可立即取消当前 Run。

完成后运行相关 Runner、Subagents、ReAct 测试及 race，再运行全量 Go test/vet。同步 `docs/设计书.md`、`DATA_MODEL.md` 与 `STATUS.md`；计划完成后删除或收口本文件。

## 不做

- 不在 `kernel/llm` 重排历史。
- 不新增耐久收件箱、下一轮队列、通用消息总线或新 RPC。
- 不修改前端消息投影和 Subagent 任务存储格式。
- 不自动修复或删除用户已有会话。
