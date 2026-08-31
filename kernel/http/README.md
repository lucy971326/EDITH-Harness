# HTTP

【它是什么】

内核插件。已经实现。它是公共 HTTP 服务器和路径登记处，不认识聊天、页面或 SSE 内容。

【提供能力】

在 Host 的 `http` 键提供 `*http.Registry`：插件可以登记标准 `net/http` 路由。

【使用能力】

不 Resolve 其他服务。

【填充插槽】

不填其他插槽；它自己提供 HTTP 路径登记处，等待 Web 等插件填入路由。

```text
Plugin.Start
  → 监听 Host:Port
  → RegisterService("http", Registry)
  → 其他插件 Register("POST /...", Handler)
  → 请求交给 ServeMux
  → Plugin.Close 立即关闭连接
```

Handler 使用实现 `ServeHTTP` 的结构体。路由随进程整体消失，不做单条注销。
