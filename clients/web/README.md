# Harness Web

正式 React 前端。页面规则见根目录 [WEB_UI](../../WEB_UI.md)，当前能力见 [STATUS](../../STATUS.md)。

## 启动

在仓库根目录运行：

```sh
make run
```

Make 会安装前端依赖、构建 `dist/` 并启动 Harness。浏览器打开 `http://127.0.0.1:8888`，业务 WebSocket 使用同源 `/rpc`。

开发页面时可另开终端运行：

```sh
cd clients/web
npm run dev
```

Vite 页面位于 `http://127.0.0.1:5173`，仅把同源 `/rpc` 代理到正式后台 `127.0.0.1:8888`。

## 阅读路线

```text
client/rpc.ts          JSON-RPC 收发
client/chat.ts         订阅、切换和重连
state/chat.ts          Snapshot + 事件 → 一份投影
state/chat-process.ts  只读派生分轮、工具配对和最终正文
chat-messages.tsx      消息列表与滚动
work-process.tsx       三级工作过程
App.tsx                连接、会话、草稿和操作流程
sidebar.tsx            项目与会话列表
composer.tsx           输入区
```

## 验证

仓库根目录的 `make test` 执行前后端测试、契约检查、网络验收、Go vet 和相关 race。隔离浏览器后台仍可运行：

```sh
HARNESS_WEB_QA=1 go test ./products/harness -run '^TestTypeScriptClient$' -count=1 -v -timeout=0
```

它使用临时数据与本机模型替身，不读取用户会话。Windows 原生目录选择器仍需在交互式 Windows 桌面人工验收。
