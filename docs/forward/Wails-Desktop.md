# Wails Desktop

状态：前沿探索，尚未进入施工。

## 目标

把现有 Harness 封装成一个桌面应用，不重写前后端。

## 形状

```text
Harness.exe
├─ Wails 窗口
├─ React
├─ appserver
└─ Host / Product / Kernel
```

## 决定

- Wails 只负责桌面窗口。
- Go 与 React 打进同一个应用。
- 应用内部监听 `127.0.0.1` 随机端口。
- 继续使用 WebSocket + JSON-RPC 2.0，不增加 Wails 业务 RPC。
- 浏览器版与 Desktop 版共用 React、契约和后台。
- 首版选择稳定的 Wails v2。

## 首版范围

- 提供 Desktop 启动和构建入口。
- 使用系统标题栏，正确处理启动与关闭。
- 暂不做托盘、自动更新、签名和跨平台发行。
