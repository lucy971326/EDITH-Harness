# runner

【它是什么】一轮对话的运行外壳插件。

【使用能力】

- `sessions`：先落账，再广播耐久消息；
- `sessionSettings`：读取本轮模型、思考档位、Agent 与工作区；
- `agents`：准备本轮输入；
- `loops`：按 Kind 运行 Loop；
- `llm`：压缩时直接 Stream；
- `tools`：压缩时取当前工具 schema；
- `events`：发布稳定 `RunEvent`；
- `sessionPersistence`：读写与账本分开的运行记录字节。

【提供能力】注册服务 `runner`：`Start`、`Run`、`Steer`、`Receive`、`Stop`、`StopRun`、`Compact`、`SessionView`，并管理活 Run。生成期草稿留在 liveRun；完整消息先落账再清除同 Entry.ID 草稿。运行结果写入 `runs.json`。准备设置前就占用 live，停止与关闭覆盖准备期；`StopRun` 只取消匹配 RunID 的运行。`Receive` 接收带来源的协作消息，以账本中的消息 ID 去重；不接收时返回待投递，不启动新 Run。

【填充插槽】不填。

【谁在用】`chat` 发起、插话，也经 `events` 接收 Run 事件；用户停止交给 `subagents.StopFamily`，由它取消父与孩子。`subagents` 还负责启动孩子，并向正在运行的父会话投递协作消息。

【不做】不自己决定怎么思考；具体推理与工具循环属于 Loop。
