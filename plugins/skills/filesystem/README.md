# skills-filesystem

【它是什么】从约定目录发现 Agent Skills 的内核 Provider 插件。

【使用能力】

- `machine`：取得主目录、读取目录和 `SKILL.md`；
- `skills`：登记自身 Provider。

【提供能力】不注册 Host 服务；提供用户目录与工作区目录中的 Skill 摘要。

【填充插槽】向 `skills` 登记处填入文件系统 Provider。

【谁在用】`agents` 在 Choices / Prepare 时动态读取发现结果。

【目录】用户侧扫描 `~/.harness/skills`、`~/.agents/skills`；工作区侧扫描
`<workspace>/.harness/skills`、`<workspace>/.agents/skills`。项目覆盖用户，
同层 `.harness` 覆盖 `.agents`。

【不做】不读取 Skill 正文交给模型、不新增 Skill Tool、不执行 Skill。
