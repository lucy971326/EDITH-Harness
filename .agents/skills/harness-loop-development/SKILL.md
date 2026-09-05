---
name: harness-loop-development
description: 在 Harness 中新增 Loop 或修改 Runner / Loop 执行流程，核对 Steer 检查点、工具调用与结果配对、取消收尾及事件落账。用于执行语义变更和相关故障修复，不用于仅改 Agent 设置、工具业务实现或聊天样式。
---

# Loop 开发与验收

让执行流程在正常结束和停止时保持对话记录完整，出错时准确返回失败。这里是开发与验收指引，不代表当前代码已覆盖所有异常场景。

## 先确认实际契约

本 Skill 位于 `.agents/skills/harness-loop-development/`；链接相对本目录，代码路径相对仓库根。

- 先读 [AGENTS.md](../../../AGENTS.md) 与 [STATUS.md](../../../STATUS.md)，本轮已读可复用；查 [设计书](../../../docs/设计书.md) 中本次相关的 Runner / Loop / 事件章节。
- 读 `kernel/loops/types.go`，确认 `Invocation`、`Emit`、`Checkpoint` 当前签名；按改动阅读 `kernel/runner/run.go` 或 `kernel/runner/steer.go`。
- 默认实现入口为 `plugins/kernel/loops/react/react.go`。参考它的调用方式，但用下面的验收条件判断正确性，不把现有实现视为天然正确。

```text
Runner → 准备本轮输入、管理运行、落账并发布事件
Loop   → 使用本轮输入、执行程序、通过 Emit / Checkpoint 协作
```

新增 Kind 的组装另读 [插件开发与组装](../harness-plugin-development/SKILL.md)；单纯修复执行逻辑不必扩展到插件组装。

## 根据改动核对运行条件

### 正常结束与 Steer

检查点返回的 Steer 已由 Runner 落账，Loop 将其纳入后续执行，不再次 Emit 为用户消息。确定继续执行时用 `CheckpointContinue`；准备正常结束时用 `CheckpointFinal`。最终检查点仍返回输入，就继续处理，不能直接结束。

停止或失败不要求继续调用模型来消耗 Steer。不要为了“取尽”而吞掉取消信号；核对 Runner 中这些已接受输入的实际保留方式。

### 工具调用与停止

以调用 ID 配对，不按工具名配对；同一个工具可调用多次。已经落账的调用，在正常完成或可完成收尾的停止路径上，都应有且只有一个对应结果。

```text
已落账调用 [A, B, C]，B 执行中停止
A → 保留已有结果
B → 已取消
C → 未执行：任务已停止
```

补写 C 的结果不等于执行 C。每个工具开始前检查取消；检查取消发生在首个调用前、当前工具中、两个调用之间的路径。当前工具取消也不代表其副作用已撤销。

工具执行沿用可取消的运行 Context；必要的落账收尾使用不被这次取消打断的 Context，不能借此继续业务执行。具体使用方式与当前 Emit 契约核对。

### 错误与通知

区分工具业务错误、运行取消、落账 / 通知失败。工具业务错误按现有 `tools.Result.IsError` 契约处理，不把所有错误一概当成取消。

`Runner.emit` 当前先 Append，再发布事件：Emit 返回错误不证明消息没写入。不能凭返回错误盲目重试或重复补结果。写入失败应返回失败；通知失败也要保留错误，不承诺磁盘失败或进程崩溃时仍能补齐全部记录。

涉及工具事件时，核对调用 ID 与 `StepSeq / BlockSeq` 的关联；收尾结果仍走原有 Emit 路径，不直接写 Session 或另外向浏览器广播。

## 最小验收

修 Bug 优先复现再修复；根据改动选择少量用例，不为每次小改动重跑全部场景：

| 改动风险 | 核心可观察结果 | 现有测试入口 |
|---|---|---|
| 多调用取消 | 已完成不重复、当前和剩余都有结果、剩余工具未执行；重新读盘仍完整 | `plugins/kernel/loops/react/react_test.go` 的多调用取消测试 |
| 最终检查点 | 最终回答附近接受的 Steer 会进入后续执行，不重复落账 | `kernel/runner/steer_test.go`、ReAct 检查点测试 |
| 落账 / 通知失败 | 未写成功的消息不发布，错误不被吞掉，已写消息不盲目重写 | `kernel/runner/run_test.go`、本次受影响的 Emit 路径 |
| 正常工具执行 | 结果配对且能继续调用模型，普通完成行为不退化 | ReAct 已有工具调用测试 |

常用局部检查：`go test ./plugins/kernel/loops/react ./kernel/runner ./kernel/session`。并发控制变化再考虑相关包 race 测试；纯后端执行修复不需要浏览器验收。

新增第二种 Loop 时，先用同样的行为条件验收；只有真实重复出现后再提炼共享测试，不预建测试框架。交付说明修复的场景、验证结果与仍未覆盖的相关边界。
