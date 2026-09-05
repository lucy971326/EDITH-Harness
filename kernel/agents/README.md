# agents

【它是什么】Agent 设置插件。

【使用能力】

- `agentStore`：读取和保存 Agent 设置，由 `persist` 提供；
- `loops`、`tools`、`skills`：校验 Kind 与普通 Tool，并取得本轮 Skill 与动态 Tool 事实。

【提供能力】注册服务 `agents`：保存设置，并在每轮 `Prepare` 时产出 Kind、Tool 名单与最终 System Prompt。当前作用域的 Skills 与 MCP 自动加入；普通 Tool 仍按 Agent 勾选。

【填充插槽】不填。

【谁在用】`runner` 每轮调用 `Prepare`，再交给选中的 Loop。

【不做】不运行 Loop、不写账本、不把提示词另存进 Session；不新增 Skill Tool。
