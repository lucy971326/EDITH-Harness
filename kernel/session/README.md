# session

拥有对话账本、元数据和分叉。`NewPersistence` 创建 JSONL / JSON 文件存储，`NewStore` 管理打开的 Session；底层字节读写交给 `persist.Files`。

`AppendID` 支持先分配身份再落账；重复 ID 报错，不覆盖。`History` 从最近摘要开始投影有效历史，未完成消息保留标记，不携带悬空工具调用。

Runner 追加对话事实，conversations 编排会话操作。这里不调用模型、不发布运行事件，不保存 UI 或其他业务状态。运行记录由 Runner 自己读写。
