# builtin skills

发布内置 Skill 摘要，并把嵌入正文写成 Agent 可读取的文件。

```text
assets/skill-creator/SKILL.md
  -> New -> persist -> system/skills/skill-creator/SKILL.md
  -> Provider.List -> skills 登记处
```

源码在 `provider.go`。当前提供 `skill-creator`；不管理用户或项目 Skill，也不把正文复制进 Agent 设置。
