# Runner + React 实现计划

目标：完成一条可运行、可测试的聊天闭环。

```text
用户输入 → Runner → Agent 设置 → Loop(react) → LLM / Tools
         ← 事件  ← 完整消息落账 ←
```

## 1. 删除 prompts

- 删除 `prompts` 登记处和相关启动项。
- Agent 配置改为 `Kind / SystemPrompt / Tools / Skills`。
- 默认 Agent 的 `SystemPrompt` 由代码内置；自建 Agent 可编辑完整文本。

验证：全仓编译通过；没有代码再 Resolve `prompts`。

## 2. loops、skills、agents

- `loops`：登记 Loop，按 Kind 取出；定义本轮 `RunConfig`、`Invocation` 和事件出口。
- `skills`：最小登记处，只提供 Skill 名和摘要。
- `agents`：保存自建 Agent；提供默认 Agent；校验 Kind、Tool、Skill 名。
- `agents.Prepare(AgentID, Workspace)`：按 Skill 名取最新摘要，拼出本轮最终 System Prompt。

验证：默认 Agent 自动取得新登记的 Skill；自建 Agent 只能使用已登记的 Kind / Tool / Skill。

## 3. events 与 Runner

- `events`：最小通知登记处；不保存数据，不替代有返回值的服务。
- Runner 每轮读取 SessionSettings 和 Agent，得到 `RunConfig`。
- Runner 追加用户输入、读取 History、选择 Loop、准备模型和已授权 Tool 入口。
- Runner 是唯一写账者：完整消息 Append 成功后，再发布 RunEvent。
- 实现 Stop、Steer 的运行时队列和 Checkpoint；FollowUp 是 Run 结束后的下一次 Run。

验证：用户输入、Steer、assistant 消息、工具结果都由 Runner 顺序写入同一本账；订阅者能收到已落账事件。

## 4. 默认 react Loop

- 插件填入 `loops` 的 `react` 条目。
- Loop 只使用 Invocation；不 Resolve Host 服务。
- 文字和思考片段实时 Emit；完整 assistant / tool 消息交 Runner 落账。
- 同一批 Tool 并行执行；结果按模型请求顺序交回模型。
- 每次 assistant 回复及整批 Tool 完成后调用 Checkpoint；有 Steer 就继续，无 Tool 且无 Steer 就结束。

验证：无 Tool 回复、并行多 Tool、单 Tool 失败、Steer、Stop 都有测试；`go test ./...` 通过。

## 提交方式

按以上四步各提交一次。每一步都先跑测试；不做真实 Skill、UI 或 HTTP 页面。
