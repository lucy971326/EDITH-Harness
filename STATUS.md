# 项目状态

更新日期：2026-09-13

## 当前形状

Harness 已完成 React Web 迁移，正式入口只有一条：

```text
http://127.0.0.1:8888/    React 静态页面
ws://127.0.0.1:8888/rpc   WebSocket + JSON-RPC 2.0

React Client
     ↓
appserver.Server
     ├─ HarnessProduct → Runner / Session
     └─ 公共服务 → Agents / LLM / Skills / Commands
```

旧 Templ / HTMX / POST / SSE 页面体系、页面插件、模拟原型与迁移施工文档已经删除，不维护双轨。

## 已完成能力

- 项目与会话：原生目录选择、按工作区分组、新建或复用空会话、切换及自动命名。
- 聊天：文字与图片、实时输出、直接 Steer、停止父子任务、历史、刷新、重连与后台重启恢复。
- 呈现：Snapshot 与事件共用一份投影；工作过程按账本结构同级展示说明、思考、工具和子任务回报，工具详情局部展开；支持 Markdown 清洗、最终回答及失败／停止状态。
- 设置：模型与思考、SessionSettings、Agent 增删改及删除保护。
- 操作：上下文用量、回答分叉、Skill 候选、命令候选与 compact。
- 辅助工作区：平级外壳、开关、调宽及标签管理；文件、浏览器、终端内容尚未接入。

## 后台边界

- appserver 是 Host 外的接入层，只管理静态页面、协议、连接、订阅与清理，不拥有业务状态。
- HarnessProduct 负责创建、发送、Steer、停止、快照、分叉和命令准入。
- Runner 负责运行、草稿、事件、取消与收尾；模型输出及整批工具结果组成不可插入的步骤，Steer 与协作回报只在随后检查点按序落账。连接断开不停止已接受的 Run。
- Stop 是独立的 Context 取消；它不进账本，并可取消仍在等待检查点的 Steer。
- 耐久消息先落账再发布；增量只进入运行投影。生成、草稿和最终 Entry 共用同一个 Entry.ID。
- 每个 Session 同时只有一个活 Run；运行状态与最近用量保存在 `runs.json`，未完成运行在重启后标记 interrupted，不自动续跑。
- Client 只保存服务端投影和草稿、主题、折叠等临时界面状态，不成为业务事实来源。

## 构建与启动

正式命令在仓库根目录执行：

```sh
make run          # 构建 React 并启动，自动打开浏览器
make build        # 产出 .build/harness
make test         # 前后端、契约、网络、vet 与相关 race
make agent-check  # Agent 日常快速回归：构建一次，其余常用检查并行
```

Vite 构建产物位于 `clients/web/dist/`，由 Go embed 进入二进制但不提交 Git。`make run` 与 `make build` 会先生成它；不再读取 `harness.yaml`，监听地址固定为 `127.0.0.1:8888`。

前端开发页仍可在 `clients/web` 运行 `npm run dev`，默认使用 `http://127.0.0.1:5173`，同源 `/rpc` 代理到正式后台的 `8888` 端口。

## 数据与配置

数据根目录固定为 `~/.harness`：

```text
~/.harness/
├─ config.yaml
├─ agents/<agent-id>.json
├─ skills/<skill>/SKILL.md
├─ system/skills/<skill>/SKILL.md
├─ sessions/<session-id>/{meta.json,messages.jsonl,settings.json,runs.json}
├─ subagents/tasks/<task-id>.json
└─ mcp.json
```

本机模型 Provider 仍在 `~/.harness/config.yaml` 配置。machine-local 直接操作本机文件和进程，没有沙箱与路径限制。

## 后续范围

- Wails 包装正式 React 页面与唯一后台的启动／退出。
- 服务端反向请求与待回答恢复。
- 有副作用操作的业务 ID 防重及完整多 Client 协调。
- 辅助工作区中的真实文件、浏览器和终端。

这些能力尚未实施，不应提前增加兼容层或通用框架。

## 已知未验证

- Windows 原生目录选择器仍需在交互式 Windows 桌面验收。
- 前端主包超过 Vite 默认 500KB 提示；当前不影响构建和运行，尚未为了消除提示引入代码分割。

## 本次验证

- `make build` 与 `make test` 通过：全量 Go test/vet、相关 race、契约检查、真实网络验收和 50 项前端测试均通过。
- 正式 `make run` 从 `8888` 加载嵌入页面；浏览器确认同源 RPC 已连接、真实会话可打开且 Markdown 正常渲染。
- 未调用付费模型、修改用户会话或执行用户工具。
