# host

> 一个很小的“服务表 + 插件生命周期管理器”。

```text
Plugin.Start
   ↓ RegisterService
Host ──Resolve──→ 其他 Plugin
   ↓ Close
按安装顺序反向关闭
```

- `host.go`：登记、解析服务和统一关闭。
- `plugin.go`：插件只需实现 `Name / Start / Close`。

Host 不懂聊天、模型或工具业务，也不负责动态加载配置。
