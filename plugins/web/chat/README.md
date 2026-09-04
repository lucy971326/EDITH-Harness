# Chat 产品

【它是什么】默认 Chat Web 产品：项目/会话导航、对话页面、History 与 SSE 实时画面。

【使用能力】

- `web`：登记 Chat 产品和 HTTP 路由；
- `sessions`、`sessionSettings`：列出会话，读取本轮配置；
- `agents`：列出可选 Agent，并在普通发送前保存此会话下一轮使用的 Agent；
- `runner`：发送、Steer、FollowUp、Stop；
- `llm`：列出模型和思考档位；
- `events`：订阅 `RunEvent` 与 `DockChanged`，转成浏览器 SSE。
- `surface/web/runview`：统一投影历史、思考、Tool 与最终回答；Chat 不维护运行 reducer。

【提供能力】注册服务 `chat`：提供 `sidepanel`、`dock`、`message.actions`、`composer.actions` 四个 Chat 内部登记处；内置 `copy` 消息动作。

【填充插槽】向 `web` 填入 Chat 产品和全部 `/chat` 路由。

【谁在用】浏览器使用 Chat 路由与 SSE；`sidepanel/*`、`dock/*`、`message/*`、`composer/*` 下的填充插件 Resolve `chat` 并登记对应条目。

【不做】不保存 Run、FollowUp、Dock 或其他插件业务状态；Session 只保存对话。

## 目录形状

```text
根目录
├─ plugin.go / handler.go / page.templ / events.go
│  Chat 产品本体：路由、页面、History 与 SSE
├─ static/
│  Composer、停止、消息动作与右侧面板的私有浏览器交互
├─ slot_composer.go / slot_dock.go / slot_message.go / slot_sidepanel.go
│  Chat 提供的四个内部插槽登记处
├─ message_copy_action.go
│  Chat 自带复制动作
├─ message/fork/
│  Chat 消息动作插槽的分叉填充者
└─ composer/ dock/ message/ sidepanel/
   外部插件填入对应插槽的位置
```
