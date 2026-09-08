# RunView

> 迁移期旧实现。React Client 将保留 Snapshot + 事件进入同一 reducer 的行为，不复用本目录的 Templ / HTMX / SSE 实现；迁移完成后删除。

【它是什么】旧 Web 运行视图：把 HarnessProduct 提供的历史快照与 Runner 实时事件画成输入、工作过程和最终回答。

【使用能力】读取 `harness.Snapshot`、`session.Entry` 与 `runner.RunEvent`；浏览器使用 HTMX SSE、`marked` 和 `DOMPurify`。

【提供内容】`View(Config)` Templ 组件和公共 `runview.js`；用户消息里的图片按块画出来。不在 Host 上登记服务，不提供插槽。

【谁在用】仅当前旧 Chat。

【不做什么】不负责发起 Run、产品路由、Composer、Dock、Sidepanel、消息动作或产品自己的状态。
