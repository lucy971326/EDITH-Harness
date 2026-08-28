# core 主干

这是 DSH 怎么装。不是我们的方案。

阅读顺序：`agent.md` → `agent-loop.md` → `tools.md` → `session.md` → `system-prompt.md` → `llm.md`。

`packages/core/` **不是一个定义者**。它是一组包：会话日志、工具登记处、对话约定、默认司机。

和 `dsh-shell` 不同：这里的定义者包 **自己也是插件**，启动时 `Provide` 服务（经常是带插槽的空登记处）。

`llm` 不在这组，在 `packages/llm/`。对话转圈时 `Get("llm")`。

---

## 各包身份

| 包 | 身份 | 干什么 |
|---|---|---|
| `scope/` | 库，不是插件 | 没有 ctx 键。给登记处做「按 agent 可见」 |
| `session/` | 定义者 + 插件 | 注册服务 `sessions`：只追加的对话日志 |
| `system-prompt/` | 定义者 + 插件 | 注册服务 `systemPrompt`：提示词插槽 |
| `tools/` | 定义者 + 插件 | 注册服务 `tools`：工具插槽 + 执行流水线 |
| `agent/` | 定义者 + 插件 | 登记处 `agents`：Roster；写出 Chat 接口、Ticket、工厂插槽、`agent/*` 事件 |
| `agent-loop/` | 司机 | 填工厂、实现 Chat、对话内部转圈。源码另占键 `agentLoop` |
| `agent-default-model/` | 插件 | 注册 `agentDefaultModel`：没选模型时用谁 |
| `agent-tool-presentation/` | 插件 | 填充 `tools` 的呈现插槽（模型看到 native 还是 code）。旁支 |

扩展插件 import 登记处包，**不** import 司机包，司机才能换。

---

## 服务表

```
┌─ Context ─────────────────────────────┐
│  sessions            日志本子           │
│  agents              登记处 Roster       │
│    工厂插槽 ← 司机填充                  │
│  agentLoop           司机另 Provide 的服务 │
│  tools               工具名单（插槽）    │
│  systemPrompt        提示词段（插槽）    │
│  agentDefaultModel   默认模型           │
│  llm                 （不在 core）      │
└───────────────────────────────────────┘
```

---

## Create / Followup 落在哪

```
入口  ctx.Get("agents").Create(...)     ← 登记处把活转给工厂，返回票
          │
          ▼
      司机造 Session + Chat
          │
入口  票.Chat.Followup(话)              ← Chat 接口写在登记处包
          │
          ▼
      收件箱 + 敲门
          │
          ▼
      这台对话转圈：问 llm、跑 tools、写 sessions
```

`Create` / `Followup` 的契约在 `agent/`。真正造对话、转圈的是司机包。

---

## 和 shell 三身份对照

```
shell:  定义者包不装；提供者 Provide("shell")
core:   session/tools/agent 自己装，Provide 登记处
        司机包填工厂插槽 + 实现 Chat
        tool-bash 等是 tools 的填充者（不在 core）
```
