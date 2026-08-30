# session

【它是什么】对话账本插件。

【提供能力】注册整份服务 `sessions`，用于创建、取得和列出账本。

【使用能力】Resolve `sessionPersistence`。

【填充插槽】不填。

## 代码主干

```text
Store.Get / Create
  → 得到一本 Session
  → Append：在当前 Head 后追加节点并写盘
  → History：沿父链读取当前分叉
  → Branch：只移动 Head
```

Session 只记对话；不负责事件通知、模型调用和 SessionSettings。
