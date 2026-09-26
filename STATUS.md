# 项目状态

更新：2026-09-26。架构决策见[设计书](docs/设计书.md)，数据位置见[DATA_MODEL](DATA_MODEL.md)。本篇只保留当前事实，不累计施工日志。

## 当前能力

- React Web：项目与会话、文字／图片聊天、实时过程、Steer、停止、分叉、压缩、上下文引用。
- Wails v3 Desktop：复用同一份 React 构建产物与后台，业务通过 Stream 传完整 JSON-RPC 2.0 消息；使用系统标题栏，关闭最后窗口即退出；同一份用户数据只允许一个后台运行。
- 设置：Agent、模型与思考档位、权限模式、智能审批、可折叠 Hooks 列表；UI 风格已由用户确认满意。
- 工作区：Monaco 编辑器、自动保存与冲突保护、文件监听、Run Diff 与受版本保护的撤销、真实 PTY 终端。
- Agent 能力：命令／持续进程、补丁、MCP、Skills；子任务支持主会话 → 孩子 → 孙子，独立页面、续聊与递归停止。
- 安全：四档权限、人工／LLM／Jev 审批、项目 MCP 配置信任与逐次工具审批；Linux bwrap + seccomp、macOS Seatbelt。
- Hooks：PreToolUse 本地命令、全局／项目设置、项目信任、顺序与超时；明确拒绝阻止工具，故障报告后放行，Run 取消停止执行。用户已验证日志 Hook。
- 后台采用入口显式装配、internal 领域组织；Host、Product、旧路径转发和旧数据迁移已移除。存储格式归领域，persist 只负责可靠文件操作；编辑器文件生命周期集中在 use-files。

## 构建与启动

需要 Go、Node.js、npm、Make；Windows 还需要 Git Bash。仓库根目录执行：

```sh
make run           # 构建前端并启动 Web
make build         # 产出 .build/harness
make desktop-run   # wails3 dev，前端热更新、Go 改动后重启
make desktop-build # 产出 .build/harness-desktop
make agent-check   # 日常检查
make test          # 完整串行验收
```

Web 页面 `http://127.0.0.1:8888/`，业务连接 `ws://127.0.0.1:8888/rpc`。Desktop 不开放业务 TCP 端口。前端 dist 由 Go embed 打包，不提交 Git。Vite 开发页使用 5173，将 `/rpc` 代理到 8888。

模型与 Jev 密钥由 `~/.harness/config.yaml` 配置；其他数据位置与所有者见 DATA_MODEL。没有旧数据迁移，升级开发版本前自行决定是否清理数据。

## 限制与未验证项

- Windows 受限 Agent 沙箱未实现，Full Access 可用；原生目录选择仍需交互式 Windows 验收。
- Desktop 已通过构建与连接入口检查；macOS 窗口聊天及现有页面交互待用户实机验收，Windows 桌面版未实机验收。关闭最后窗口即退出，暂不做签名、安装包或托盘。
- macOS Seatbelt 已在 macOS 27.0 arm64 验证，其他版本与 Intel 未验证。Linux 需要支持相应 namespace、seccomp 与挂载能力的 bwrap。
- 沙箱不隔离硬链接别名；受保护目录内部的细粒度子路径授权暂时拒绝。异常断电／强杀可能留下 Linux 挂载占位空目录，不自动删除来源不明目录。
- MCP 与 Hook 在宿主或远端执行，不在 Agent 命令沙箱内；Hook 信任仅覆盖配置，不跟踪脚本内容。
- LLM／Jev 已有用户实际使用反馈，但自动审核的安全准确率没有完成生产校准。
- 上下文引用的中文输入法／窄屏、嵌套子任务的实机流式交互及真实 MCP Server 交互，尚无完整验收记录；总体 UI 满意不替代这些专项验证。
- Vite 仍提示较大的主包／Monaco chunk。Monaco 使用本地资源并按需加载。
- Windows 管理员环境曾因只读位不能模拟写入失败而导致 subagents 测试失败，尚无该环境修复后的验证记录。

## 后续方向（未实施）

headless CLI、通用服务端反向请求、业务操作防重与完整多 Client 协调、辅助浏览器、Windows 沙箱。新的 Hook 事件仅在有具体需求时设计，不预建框架。

现有 Web 与 Desktop 的共用边界见[设计书](docs/设计书.md)。

## 最近验证

- 2026-09-26 新会话默认模型只选密钥可用的 Provider；无可用密钥时创建失败并清理空会话。`make agent-check`、`wails3 task deps` 和 `wails3 task build` 通过；Desktop 窗口交互仍待用户实机验收。
- 2026-09-25 Wails 开发模式：`wails3 task build:dev`、`make agent-check` 通过；Vite 按 `WAILS_VITE_PORT` 启动且返回 HTTP 200。窗口内热更新仍待实机确认。
- 2026-09-25 Desktop 第一步：`wails3 doctor` 就绪，`make agent-check`、`wails3 build`、Windows 目标 Go 交叉编译通过；桌面二进制文件助手模式通过。连接入口与数据目录独占锁有核心测试；窗口聊天待用户实机验收。
- 2026-09-24 工程目录整理：`make agent-check` 通过，包括前端构建、手写契约／RPC 类型检查、70 项前端测试、Go 测试（含网络验收）、vet 与指定包 race。资源随包迁移，未新增测试；此次未做浏览器视觉验收。
- 本次文档收口：删除已完成计划与过时教学稿，核对核心决策、源码入口及本地链接；不修改代码，不重复运行构建与测试。
