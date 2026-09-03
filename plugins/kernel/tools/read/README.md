# read

【它是什么】读取文本文件的 Tool 插件。

【使用能力】

- `machine`：从工作区读取文件，通常由 `machine-local` 提供；
- `tools`：登记自身。

【提供能力】不注册整份服务；提供 `read` Tool。

【填充插槽】向 `tools` 登记 `read`。

【谁在用】`react` 通过 `tools` 在模型请求时调用它。

【不做】不自行选择文件、不保存读取结果，也不绕过 Tool 允许名单。
