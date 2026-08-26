# loop

一句话：**会话搬运工**，把一条用户消息跑成“问模型 → 调工具 → 再问模型”的完整回合。

```text
agents 把 Conversation 交给 Runner
                │
                ▼
               loop
                │
      inbox → driver → step
                │       │
             session   llm + tools
```

- 填充插槽：向 `agents` 登记 `Runner`；不提供新的全局服务。
- `turn` 是一次用户问题；`step` 是一次 LLM 请求及其工具调用。
- 每一步先记账、再请求；取消与崩溃恢复都以账本为准。
- 先读：`plugin.go` → `runner.go` → `conversation.go` → `driver.go` → `step.go`。
