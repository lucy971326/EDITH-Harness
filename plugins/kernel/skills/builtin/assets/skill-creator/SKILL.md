---
name: skill-creator
description: 创建或修改 Harness Skill，并把它放到正确的用户级或项目级目录。
---

# Harness Skill Creator

用于创建或修改 Harness Skill。

## 规则

- Skill 名称使用小写字母、数字和单个连字符，目录名与 `name` 完全相同。
- 用户级 Skill 放在 `~/.harness/skills/<name>/SKILL.md`。
- 项目级 Skill 放在 `<workspace>/.harness/skills/<name>/SKILL.md`。
- `SKILL.md` 必须有 `name` 和 `description` 两项 YAML frontmatter。
- 先读取项目的 `AGENTS.md` 和相关文档，再写入 Skill；不要覆盖用户已有改动。
- 只写完成任务需要的规则，避免把普通说明堆进 Skill。

## 流程

1. 判断这是新建还是修改，并确认 Skill 的作用域。
2. 检查目标目录和现有 `SKILL.md`。
3. 使用 Harness 的 `write` 或 `edit` 工具写入内容。
4. 用 `read` 重新检查文件，确认 frontmatter、目录名和说明正确。
