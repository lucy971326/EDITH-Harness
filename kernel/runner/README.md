# runner

【它是什么】一轮对话的运行外壳插件。

【使用能力】

- `sessions`：先落账，再广播耐久消息；
- `sessionSettings`：读取本轮模型、思考档位、Agent 与工作区；
- `agents`：准备本轮输入；
- `loops`：按 Kind 运行 Loop；
- `llm`：压缩时直接 Stream；
- `tools`：压缩时取当前工具 schema；
- `events`：发布稳定 `RunEvent`。

【提供能力】注册服务 `runner`：`Start`、`Run`、`Steer`、`Stop`、`Compact`，并管理活 Run。

【填充插槽】不填。

【谁在用】`chat` 发起、插话、停止和排队后续对话；`chat` 也经 `events` 接收它发布的 Run 事件。

【不做】不自己决定怎么思考；具体推理与工具循环属于 Loop。
