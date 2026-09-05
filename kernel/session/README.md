# session

【它是什么】对话账本插件。

【使用能力】Resolve `sessionPersistence`：读写账本节点与会话元数据，由 `persist` 提供。

【提供能力】注册服务 `sessions`：创建、取得、列出、读取当前分叉，以及移动分叉 Head。`History()` 从最近摘要开始投影发给模型的有效历史。

【填充插槽】不填。

【谁在用】`runner` 追加对话事实；`chat` 创建、列出和展示会话。

【不做】不调用模型、不发布事件、不保存 UI、Todo 或其他业务状态。Session 只记对话。
