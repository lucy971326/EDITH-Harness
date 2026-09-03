# Dock 动态演示

【它是什么】

只用于自动化测试的 Dock 填充插件，不会安装到正常的 Harness。

【提供能力】

不注册 Host 服务；向 Chat 的 Dock 登记处填入 `demo` 条目。

【使用能力】

Resolve `chat` 与 `events`。内存按 Session 保存一个计数；`Increment` 成功改变计数后发布 `DockChanged`。

【填充插槽】

填充 `chat.Service.RegisterDock`。它返回 `templ.Component`，由 Chat 经既有 SSE 发送 HTML，HTMX 替换内容。
