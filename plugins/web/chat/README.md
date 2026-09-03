# Chat 产品

【它是什么】

Chat Web 产品插件。提供项目 / 会话导航、真实消息区和 SSE 实时画面。

【提供能力】

在 Host 的 `chat` 键提供 Chat 专属的右侧面板、Dock 和消息动作登记处。Chat 负责 Tab 壳、Dock 折叠外壳、动作路由与通用卡片规则。

【使用能力】

Resolve `web`、`sessions`、`sessionSettings`、`llm`、`runner` 和 `events`。Chat 只发 Runner 命令、订阅 `RunEvent` 与 `DockChanged`；不保存 Run、FollowUp 或 Dock 业务状态。

【填充插槽】

向 `web` 填入 Chat 产品和页面、History、SSE、发送、停止等 HTTP 路由。

## 目录形状

```text
根目录
├─ plugin.go / handler.go / page.templ / timeline.go / events.go
│  Chat 产品本体：路由、页面、History 与 SSE
├─ slot_composer.go / slot_dock.go / slot_message.go / slot_sidepanel.go
│  Chat 提供的四个内部插槽登记处
├─ message_copy_action.go
│  Chat 自带的复制动作
└─ composer/ dock/ message/ sidepanel/
   外部插件填入对应插槽的位置
```

## 右侧面板

```text
面板插件 → chat.Service.RegisterPanel(类型)
浏览器   → type + instance
Chat     → 查已登记定义 → Panel.Render(templ.Component)
```

面板 Tab 的身份是 `(Type, InstanceKey)`：相同身份只聚焦，不同身份新开。Tab、展开状态与宽度仅存浏览器当前页面；刷新或切换会话即丢弃，Session 不记录面板状态。工具调用仍在消息工具卡中。

## 消息动作

```text
动作插件 → chat.Service.RegisterMessageAction(动作)
paint()   → 已落账消息卡片下方画标准按钮
POST      → Chat 查当前分叉账本 → 动作 Execute → JSON
```

内置 `copy` 动作同时用于用户和助手卡。助手卡以 `RunID + boundaryEntryID` 定位：从该用户消息开始，到下一条同一 Run 的用户消息停止，取其中最后一条有正文的助手消息。因此 reasoning、工具块、工具前的临时文字和相邻 Steer 段都不会进入剪贴板；运行中的助手卡不显示复制。

## Dock

```text
业务插件 → chat.Service.RegisterDock(条目)
状态提交 → events.Publish(DockChanged)
Chat     → Dock.Render() → 同一 SSE 的 dock-{id} HTML
HTMX     → sse-swap 替换该条目的内部内容
```

Dock 是 Chat 输入框上方的 `list` 插槽，不是插件，也不保存 Todo、Goal、Subagent 等业务状态。Chat 固定画 `<details>` 折叠外壳和标题；填充插件只返回 `templ.Component`。浏览器保留展开状态，首次连接与重连都会得到全部 Dock 的当前 HTML。未登记条目不发送；单个渲染失败只显示“暂时无法加载”，不会影响对话 SSE。

## 实时对话

```text
GET History Snapshot JSON ─┐
                           ├→ apply() → paint() → 消息区
RunEvent → SSE JSON ───────┘

POST Run / Steer / FollowUp / Stop → Runner
```

一张会话页保持一条 SSE；服务端先订阅再确认连接，重连时 History Snapshot 同时带仍在运行的 Run，实时 Delta 才有稳定落点。Dock 也复用这条连接，但直接以 HTML `sse-swap` 更新，不进入聊天 JSON reducer。慢客户端只会被断开，不能阻塞 Runner。

同一 Run 默认合为一张助手卡，按 `StepSeq / BlockSeq` 保留思考、正文与工具的顺序。只有 Runner 已把 Steer 用户消息落账时，才以该用户 Entry 为边界把同一 Run 切成前后两个片段；助手消息完成落账不会改变片段位置。模型和思考档位由 `llm.Client.Models()` 提供，首次发送前必须选好。
