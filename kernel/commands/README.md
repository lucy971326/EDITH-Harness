# commands

【它是什么】平台命令登记处插件。

【使用能力】不 Resolve 其他服务。

【提供能力】注册登记处服务 `commands`：登记命令，按名 `Call`。

【填充插槽】自身不填；`compact` 等命令插件向它登记。

【谁在用】命令插件填入条目；appserver 向 Client 列出 `/` 候选，`HarnessProduct` 在准入后执行命令。

【不做】不认识 `/`，不画页面，不自己压会话。
