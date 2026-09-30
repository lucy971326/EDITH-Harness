# session

一本对话账及其元数据；不保存运行、审批或 UI 状态。

```text
Store -> 同一 ID 的 Session -> Append / History
  |                              |
  +----------> jsonl ------------+
                -> persist -> meta.json / messages.jsonl
```

- `types.go`：消息、节点、元数据与 Persistence 契约。
- `store.go`：创建、打开、按 ID 读取元数据、列表、手动改名、归档标记与分叉。
- `session.go`：追加节点、分支历史、摘要覆盖边界与保留原文引用的有效输入（`ContextEntries / History`）。
- `jsonl.go / id.go`：文件格式与身份生成。

`AppendID` 支持先分配身份再落账，重复 ID 报错。手动改名写入 `meta.json` 的 `titleEdited`，防止首条消息覆盖标题；归档只更新元数据。分叉复制指定边界前的账本；设置和运行记录由上层分别复制。这里不调模型、不发布运行事件。

```text
用户 → 助手 tool-call → tool-result → 助手正文
              同一 ToolCall.ID 配对
```

每个账本节点有自己的 ID、parent、seq 和 body；parent 表示分支关系，ToolCall.ID 只配对调用与结果，不代替消息 ID。数据格式与恢复边界见 [DATA_MODEL](../../docs/AGENTS/DATA_MODEL.md)。
