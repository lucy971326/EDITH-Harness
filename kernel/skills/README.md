# skills

【它是什么】Skill 名称与摘要登记处插件。

【提供能力】注册登记处服务 `skills`。

【使用能力】无。

【填充插槽】自身不填；未来 Skill 插件来登记摘要。

## 代码主干

```text
Register(Name + Summary)
  → 按名称登记

List          → 列出全部 Skill
Get(names)    → 按 Agent 名单取摘要
```

这里只保存摘要，不加载 Skill 正文，也不直接拼 System Prompt。
