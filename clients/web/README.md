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

以下路径相对 `src/`：

```text
App -> 页面 / 草稿 / 操作
  -> client/rpc -> WebSocket -> appserver
快照 + 事件 -> state/chat -> 聊天 / 过程 / 子任务页面
workspace/workspace-tabs -> editor / review / terminal / subagent
```

- `App.tsx`：页面装配；`workspace/sidebar.tsx / chat/composer.tsx`：会话导航与输入。
- `client/rpc.ts`：类型化 RPC；`client/chat.ts`：主会话连接；`client/run-subscription.ts`：共用订阅、补快照与清理。
- `state/chat.ts / state/chat-process.ts`：事件归并与只读分轮；`chat/chat-messages.tsx / chat/work-process.tsx`：呈现结果和过程。
- `workspace/workspace-tabs.tsx / workspace/auxiliary-panel.tsx`：辅助面板与标签编排；[`editor/`](src/editor/README.md)：文件草稿、保存、冲突和监听。
- `review/ / terminal/ / subagent/`：Diff、终端与子任务工作页。
- `settings/settings-page.tsx / settings/approval-settings.tsx / settings/hook-settings.tsx`：设置；`styles.css / components/ui/`：视觉规则与基础控件。

Client 保存界面状态和服务端投影，账本与 Run 仍以后台为准。关闭标签或断线不停止后台任务。

## 验证

仓库根目录的 `make test` 执行前后端测试、契约检查、网络验收、Go vet 和相关 race。隔离浏览器后台仍可运行：

```sh
HARNESS_WEB_QA=1 go test ./tests/integration -run '^TestTypeScriptClient$' -count=1 -v -timeout=0
```

它使用临时数据与本机模型替身，不读取用户会话。Windows 原生目录选择器仍需在交互式 Windows 桌面人工验收。
