# filesystem skills

扫描约定目录，解析 `SKILL.md` 的元数据并返回摘要。

```text
项目 .harness/skills -> 项目 .agents/skills
  -> 用户 .harness/skills -> 用户 .agents/skills
  -> 同名保留先发现者 -> Skill 摘要
```

源码在 `provider.go`：`New` 接收 machine 与 persist；`List` 确定来源顺序，`readSkill` 解析和校验。

Harness 自有用户目录走 persist，其余目录走 machine。只返回名称、描述、位置和作用域，不把正文注入模型、不执行 Skill。
