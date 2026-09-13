# write

【它是什么】完整写入文本文件的 Tool 插件。

【使用能力】

- `machine`：解析工作区路径并写入文件，通常由 `machine-local` 提供；
- `tools`：登记自身。

【提供能力】不注册整份服务；提供 `write` Tool。

【填充插槽】向 `tools` 登记 `write`。

【谁在用】`react` 通过 `tools` 在模型请求时调用它。

【不做】不管理文件版本；已有文件会由 `machine` 按调用参数整体写入。
