# agents

【它是什么】Agent 设置插件。

【提供能力】注册整份服务 `agents`，管理默认与自建 Agent。

【使用能力】Resolve `agentStore`、`loops`、`tools`、`skills`。

【填充插槽】不填。

## 代码主干

```text
Save
  → 校验 Kind / Tool / Skill 名
  → 写入 agentStore

Prepare(AgentID, Workspace)
  → 读取实时 Agent 设置
  → 取得最新 Skill 摘要
  → 拼 SystemPrompt + Skills + Workspace
  → 返回 Kind + Tool 名单 + 最终提示词
```

默认 Agent 实时使用当前全部 Tool 和 Skill；自建 Agent 使用所选子集。
