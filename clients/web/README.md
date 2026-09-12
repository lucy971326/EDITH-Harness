# Harness Web

正式 React 前端。当前完成范围与验收记录见根目录 [STATUS](../../STATUS.md)；页面交互规则见 [WEB_UI](../../WEB_UI.md)。

## 开发

先启动后台（仍打开旧 Web，并监听 RPC）：

```sh
go run ./cmd/harness
```

RPC 在 `ws://127.0.0.1:8889/rpc`。再启动本目录的开发页：

```sh
cd clients/web
npm ci
npm run dev
```

开发页默认 `http://127.0.0.1:5173`。浏览器只连接当前页面同源的 `/rpc`，由 Vite 代理到 `127.0.0.1:8889`。代理先核对原始 Origin 与当前页面 Host，通过后才改写成后台地址；开发和 preview 同一套守卫。不要关闭后台 Origin 校验。

```sh
npm run build
npm test
```

类型检查后产出 `dist/`。根目录构建入口尚未改为 embed 这套产物；`go run ./cmd/harness` 仍打开旧 Web。

## 阅读路线

```text
client/rpc.ts      收发请求与通知
client/chat.ts     当前订阅、切换和重连
state/chat.ts      Snapshot + 事件 → 一份投影
state/chat-process.ts  从投影只读派生分轮、工具配对、最终正文
chat-messages.tsx  消息列表与滚动
work-process.tsx   三级展开与复制
message-markdown.tsx  Markdown 与清洗
App.tsx            连接、会话、草稿和发送／停止流程
sidebar.tsx        项目列表展示与回调
composer.tsx       输入区展示与回调
```

以上文件位于 `src/`。模型与思考菜单在 `src/model-menu.tsx`；已选会话需模型和思考均有效才能首次发送。运行中直接插话，无等待队列。

## 不使用真实数据或付费模型的验收

在项目根目录运行 `npm run rpc:test`，会自动创建并关闭临时 Host／Runner／本地模拟模型，同时验收 `clients/test` 和正式 Web 的连接、投影。无需启动普通后台。

手工检查页面时，先确保 8889 没有普通后台，再在项目根目录启动隔离后台：

```sh
HARNESS_WEB_QA=1 go test ./products/harness -run '^TestTypeScriptClient$' -count=1 -v -timeout=0
```

再启动 Vite。选择「浏览器验收」里的会话与模型／思考；普通文字立即返回固定测试回答，输入 `hold` 生成 `waiting` 后等待，可插话／停止／刷新。测试日志打印本机控制 URL：`/qa/release` 让等待中的模型继续；`/qa/restart` 关闭并重新创建 Host／Runner，沿用隔离账本，检查自动恢复。这是正常关闭重启，不冒充进程崩溃验收。退出测试进程会清理临时数据；这些控制口不进入正式后台。

图片展示／发送、Agent 管理、用量和分叉／命令等仍按迁移计划接入；辅助工作区仍只有标签壳。有附件时整条发送被阻止。正式入口切换与旧 Web 清理不在本步。

第 4 步的独立浏览器验收页：启动 Vite 后打开 `/test/process-browser.html`。它直接使用正式消息组件和固定 Snapshot，不连接后台、不进入生产构建。检查：展开工具组和 read 详情 → 点击文字增量（阅读位置不被抢走）→ 正常完成（过程收起、Markdown 答案只一份）；重置后分别检查停止中、失败、停止、中断、亮暗与窄屏。复制失败应就地提示，不静默成功。此页不能替代真实网络验收。

## Windows 原生目录取消验收

在交互式 Windows 桌面的项目根目录，用 PowerShell 运行：

```powershell
$env:HARNESS_TEST_NATIVE_PICKER = '1'
go test ./appserver -run TestWindowsPicker -count=1 -timeout=20s
Remove-Item Env:HARNESS_TEST_NATIVE_PICKER
```

测试检查预先取消、短时取消、显示期间取消及定时器状态清理；需要真实桌面，不把其他平台的交叉编译当作验收通过。另需人工检查选目录成功、用户取消、点击确定同时断线，以及关闭后台时弹窗退出；取消后不能创建会话或残留弹窗。修改后的原生行为尚待 Windows 实机验收。
