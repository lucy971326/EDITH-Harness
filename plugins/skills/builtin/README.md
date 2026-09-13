# builtin skills

> 把随 Harness 发布的系统 Skill 提供给 Skill 登记处。

系统 Skill 来自嵌入资源，经 `persist` 的 `system/skills` 作用域读取；当前包含 `skill-creator`。

它不管理用户 Skill，也不把 Skill 复制进 Agent 设置。用户和项目 Skill 由 filesystem provider 发现。
