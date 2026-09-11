# Harness Web

正式 React 前端。第 2 步已接通真实项目与会话：初始化连接、列出会话、原生选择目录、创建／复用空会话、切换查询。聊天发送、历史和运行订阅尚未接入。

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

## 目前可用

- 连接状态：正在连接、已连接、失败／已断开；可手动重新连接。
- 打开项目：原生选择目录后调用 `harness/session/create`。取消不报错、不创建会话。
- 项目按 `settings.workspace` 分组，组内按创建时间倒序；项目旁「＋」创建或复用该目录的空会话。
- 点击会话调用 `harness/session/get`，更新标题和会话设置投影。
- 文字草稿和图片预览按会话保留在前端内存；未选中时的临时草稿单独保存。切换聊天／设置不丢草稿。

## 目前不可用

- 发送、Steer、停止、历史、运行订阅、自动重连。
- 图片发送、分叉、Skill、命令、Agent CRUD、模型修改。
- 选中会话后只提示「聊天内容将在下一步接入」，不把已有历史显示成空会话。

## Windows 原生目录取消验收

在交互式 Windows 桌面的项目根目录，用 PowerShell 运行：

```powershell
$env:HARNESS_TEST_NATIVE_PICKER = '1'
go test ./appserver -run TestWindowsPicker -count=1 -timeout=20s
Remove-Item Env:HARNESS_TEST_NATIVE_PICKER
```

测试检查预先取消、短时取消、显示期间取消及定时器状态清理；需要真实桌面，不把其他平台的交叉编译当作验收通过。另需人工检查选目录成功、用户取消、点击确定同时断线，以及关闭后台时弹窗退出；取消后不能创建会话或残留弹窗。修改后的原生行为尚待 Windows 实机验收。
