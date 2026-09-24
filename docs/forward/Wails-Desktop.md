# Wails Desktop

状态：前沿探索，尚未进入施工。多端共用边界见[多 Client 方向书](../多client%20计划书.md)。

## 目标

把现有 Harness 封装成一个桌面应用，不重写前后端。

## 形状

```text
Harness.exe
├─ Wails 窗口
├─ React
├─ appserver
└─ 显式组装的领域服务
```

## 决定

- Wails v3 负责桌面窗口和 Stream 消息传输，不承载业务判断。
- Go 与 React 打进同一个应用。
- 桌面业务消息走 Wails Stream，不为此开放回环端口。
- React WebView 的 Stream 适配放在共用前端；桌面 Go 入口把 `StreamConn` 接入 appserver 的通用连接入口。appserver 不依赖 Wails。
- 保留完整 JSON-RPC 2.0 消息格式；Stream 只提供通用消息传输，不绑定业务方法。
- 浏览器版与 Desktop 版共用 React、契约和后台。
- 首版选择 Wails v3，固定经验证的 Beta 版本；重点验证连接顺序、断线、背压和窗口关闭。

## 首版范围

- 提供 Desktop 启动和构建入口。
- 使用系统标题栏，正确处理启动与关闭。
- 暂不做托盘、自动更新、签名和跨平台发行。
