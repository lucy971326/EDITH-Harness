# session

一本对话账及其元数据；不保存运行、审批或 UI 状态。

```text
Store -> 同一 ID 的 Session -> Append / History
  |                              |
  +----------> jsonl ------------+
                -> persist -> meta.json / messages.jsonl
```

- `types.go`：消息、节点、元数据与 Persistence 契约。
- `store.go`：创建、打开、列表、改名与分叉。
- `session.go`：追加节点、分支历史、摘要后的有效输入。
- `jsonl.go / id.go`：文件格式与身份生成。

`AppendID` 支持先分配身份再落账，重复 ID 报错。分叉复制指定边界前的账本；设置和运行记录由上层分别复制。这里不调模型、不发布运行事件。
