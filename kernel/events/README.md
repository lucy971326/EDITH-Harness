# events

【它是什么】进程内同步事件登记处插件。

【使用能力】不 Resolve 其他服务。

【提供能力】注册登记处服务 `events`：监听者可 `Subscribe`，发布者可 `Publish` 同一种 Go 事件。

【填充插槽】自身不填；监听者填入的是回调，不是长期业务数据。

【谁在用】`runner` 发布稳定 `RunEvent`；当前旧 `chat` 订阅并转成 SSE。目标架构由 app-server 按 Client 订阅投影运行事件，不改变 events 的进程内职责。

【不做】不保存、不缓冲、不重放事件；有返回值的协作仍直接调用服务。
