# Web

【它是什么】

可选表面插件。已经实现阶段 1：提供通用 Web 舞台，不认识 Chat、Runner 或 Session。

【提供能力】

在 Host 的 `web` 键提供产品与 HTTP 路由登记处，并管理一台 `net/http.Server`。

【使用能力】

不 Resolve 其他服务。

【填充插槽】

不填其他插槽；它提供 `products` 与 `routes` 两个登记口，等待产品插件填入。
