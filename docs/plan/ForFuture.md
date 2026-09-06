# App Server 方向

尚未实施。本轮已把聊天业务收敛到 `ChatService`；未来才在它前面增加 App Server，不把 App Server 写成当前能力。

## 未来接入规则

未来 Web、VS Code 扩展、TUI 是对称的客户端：都经 App Server 使用业务、查询数据、接收通知。Web 没有绕过 API 的业务特权。

```text
Web 客户端 -----------+
VS Code 扩展 ---------+--> App Server --> ChatService --> 现有内核服务
TUI 客户端 -----------+          |
                                +--> 其他产品自己的业务
```

App Server 不取代 Host，服务之间仍是进程内 Go 调用。对称的是接入规则，不要求各端具备相同界面或产品功能。协议、认证、网络边界与 Web 最终展示适配尚未决定，也均未实现。
