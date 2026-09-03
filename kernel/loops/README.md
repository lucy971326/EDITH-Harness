# loops

【它是什么】运行范式登记处插件。

【使用能力】不 Resolve 其他服务。

【提供能力】注册登记处服务 `loops`：按 Kind 保存可复用的 `Loop` 程序。

【填充插槽】自身不填；`react` 等运行范式插件向它登记。

【谁在用】`agents` 校验 Agent 选用的 Kind；`runner` 按本轮 Agent 的 Kind 取出并运行 Loop。

【不做】不拥有某场 Run；活 Run 永远由 `runner` 管理。
