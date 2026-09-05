# Agent 设置插件

【它是什么】Web 公共设置页中的 Agent 管理栏目。

【使用能力】Resolve `agents` 读取、保存和删除 Agent；Resolve `web` 登记设置栏目与表单路由。

【提供能力】不注册服务。

【填充插槽】填 `web` 的 `settings.section`，栏目 ID 为 `agents`。

【谁在用】浏览器通过 `/settings/agents` 路由管理 Agent；Chat 在下一轮 Run 前读取用户保存的 Agent 选择。

【不做】不拥有 Session、Runner、Tool 或 Skill；不在 Agent 里勾选 Skills / MCP；不绕过 `agents` 服务直接读写 Agent 文件。
