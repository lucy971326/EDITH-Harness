# RunView

【它是什么】可选的 Web 运行视图：把 ChatService 提供的历史快照与 Runner 实时事件画成输入、工作过程和最终回答。

【使用能力】读取 `chat.Snapshot`、`session.Entry` 与 `runner.RunEvent`；浏览器使用 HTMX SSE、`marked` 和 `DOMPurify`。

【提供内容】`View(Config)` Templ 组件和公共 `runview.js`；用户消息里的图片按块画出来。不在 Host 上登记服务，不提供插槽。

【谁在用】Chat；未来需要展示 Agent 运行过程的 Web 产品可以直接使用。

【不做什么】不负责发起 Run、产品路由、Composer、Dock、Sidepanel、消息动作或产品自己的状态。
