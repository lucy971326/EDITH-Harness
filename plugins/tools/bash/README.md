# bash

【它是什么】执行 Bash 命令的 Tool 插件。

【使用能力】

- `machine`：在工作区启动进程，通常由 `machine-local` 提供；
- `tools`：登记自身。

【提供能力】不注册整份服务；提供 `bash` Tool。

【填充插槽】向 `tools` 登记 `bash`。

【谁在用】`react` 通过 `tools` 在模型请求时调用它。

【不做】不把非零退出伪装成成功；它会作为 Tool 错误交还模型。
