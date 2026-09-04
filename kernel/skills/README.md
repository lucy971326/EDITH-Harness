# skills

【它是什么】Skill Provider 登记处插件。

【使用能力】不 Resolve 其他服务；文件系统 Provider 自己使用 `machine`。

【提供能力】注册登记处服务 `skills`：登记 Provider，并按工作区动态发现、稳定合并 Skill 摘要。

【填充插槽】自身不填；Skill Provider 向它登记 `List(workspace)`。

【谁在用】`agents` 读取用户选中的 Skill 与工作区全部项目 Skill，拼进本轮最终 System Prompt。

【不做】不加载 Skill 正文、不执行 Skill、不自己拼完整提示词；跨 Provider 同名直接报错。
