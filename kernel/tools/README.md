# tools

【它是什么】Tool 登记处插件。

【使用能力】不 Resolve 其他服务。

【提供能力】注册登记处服务 `tools`：登记普通 Tool 与动态来源、生成 Schema、按本轮允许名单校验并调用 Tool。

【填充插槽】自身不填；`read`、`write`、`edit`、`bash` 等 Tool 插件和 MCP 来源向它登记。

【谁在用】`agents` 校验普通 Tool 名单并在 Prepare 时合并动态 Tool；`react` 取得 Schema 并在模型要求时调用 Tool。

【不做】不直接读写文件或运行命令；这些由具体 Tool 通过 `machine` 完成。
