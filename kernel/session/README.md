# session

【它是什么】对话账本插件。

【提供能力】注册整份服务 `sessions`，用于创建、取得和列出账本。

【使用能力】Resolve `sessionPersistence`。

【填充插槽】不填。

## 代码主干

```text
Store.Get / Create
  → 得到一本 Session
  → Append：在当前 Head 后追加节点、分配不复用的 Seq 并写盘
  → 首条用户消息成功落账后，把「新对话」自动命名
  → History：沿父链读取当前分叉
  → Branch：只移动 Head
```

元数据文件是会话存在的依据；空会话没有账本文件也能 Get/List。`Entries()` 提供当前分叉的 `ID / ParentID / Seq / Message` 投影；缺少 `Seq` 的旧账本明确拒绝加载，不静默混用。Session 只记对话；不负责事件通知、模型调用和 SessionSettings。
