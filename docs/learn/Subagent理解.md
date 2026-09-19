# Subagent 理解

## 面试时可以这样讲

> 我们没有把 Subagent 做成另一套执行器，而是把它定位成多会话的协作层。Runner 负责一个 Session 的一轮执行：准备 Agent、运行 Loop、写账本、处理停止。Subagents 的职责比 Runner 高一层，它组织多个 Session 之间的委派、回报和停止。HarnessProduct 则更高一层，负责用户怎样创建会话、发送消息、停止和打开页面。这样三者的边界很清楚：Product 管用户业务，Subagents 管协作关系，Runner 管单轮执行。
>
> 具体来说，主 Agent 调用 `subagent_spawn` 后，Subagents 会先把父子关系保存下来，再创建一个独立的子 Session 和它的运行设置，最后调用现有的 Runner 启动孩子。因此 `spawn` 成功返回，只代表孩子已经启动，不代表任务已经完成。孩子拿到的委派说明，在它自己的账本里是一条 `user` 消息；因为对它而言，这就是本轮要完成的任务。
>
> 孩子完成后，结果先写进孩子自己的账本。Subagents 再从子账本和 Runner 的记录中投影出结果，封装成带来源和稳定 MessageID 的 `collaboration` 消息，交给父 Run。父 Run 只会在安全检查点把这条回报写进父账本。所以结果没有被硬塞进父会话，也不会打断一段正在进行的模型输出。
>
> 这套设计的重点是让事实各有唯一归属：`task.json` 只保存一条直属父子关系；子账本保存孩子的聊天和结果；直属父账本中的 MessageID 证明回报已经送达；活跃 Run 只属于 Runner。我们没有另存一份任务状态或结果，避免双写，也让重启后的恢复更明确。当前固定允许主会话第 0 层委派到子任务第 1 层，第 1 层再委派到孙任务第 2 层；第 2 层不能继续委派。深度由父子关系计算，不写入 `task.json`，停止则沿关系递归到目标的全部后代。

```text
HarnessProduct  用户怎样使用聊天
Subagents       多个 Session 怎样协作
Runner          一个 Session 怎样执行
```

## `task.json` 长什么样

每派出一个直属孩子，Subagents 会保存一份关系文件：`<taskID>.json`。孩子继续委派孙任务时，会再保存一份以孩子为直属父会话的关系文件；每份文件始终只表达一条边。

```json
{
  "version": 1,
  "id": "task-8c1d…",
  "parentSessionID": "session-parent…",
  "childSessionID": "session-child…",
  "taskName": "检查测试",
  "description": "检查测试是否通过，并汇报发现。"
}
```

把单份文件读成一句话就够了：

```text
父会话 parentSessionID
  派出了名叫 taskName 的任务
  给子会话 childSessionID
  任务内容是 description
```

| 字段 | 是什么 |
| --- | --- |
| `version` | 文件格式版本；当前是 `1`。 |
| `id` | 这次委派的任务 ID，也是文件名的一部分。 |
| `parentSessionID` | 谁派出的孩子。 |
| `childSessionID` | 孩子自己的普通 Session。 |
| `taskName` | 给人看的稳定短名称；新委派必填，旧 v1 记录缺失时回退到 Agent 名称。 |
| `description` | 首次发给孩子的任务内容。 |

这里**没有**子任务状态、Run ID、模型设置、聊天记录、结果正文或“是否已投递”。

```text
task.json  → 只回答：父子是谁
子账本     → 结果和聊天事实
Runner     → 活着的 Run
父账本     → 回报是否已送达
```

所以程序重启后，先靠 `task.json` 找回父子关系；再分别读取两本账本和 Runner 记录，重新拼出任务现在的状态。

多层委派只是把这些直属关系连起来，不引入另一套任务树存储：

```text
主 Session（第 0 层）
  └─ task-A.json → 子 Session（第 1 层）
                       └─ task-B.json → 孙 Session（第 2 层）

每个孩子仍是普通 Session
每份 task.json 仍只保存一条直属关系
回报仍只进入直属父账本
```
