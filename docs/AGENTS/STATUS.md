# 项目状态

更新：2026-09-30。这里只记录当前能力、限制和影响使用的验证结论；架构见[设计书](../设计书.md)，持久化见[DATA_MODEL](DATA_MODEL.md)，构建命令见[项目 README](../../README.md)与[打包说明](../../build/README.md)。

## 当前能力

- Web 与 Windows／macOS Desktop 共用 React 页面和 Go 后台；本机同一份 `~/.harness` 只允许一个后台。Desktop 支持单实例、窗口恢复、关窗驻留托盘、通知；Windows 自绘标题栏，macOS 保留原生窗口控件。Linux 只提供 Web。
- 会话支持文字／图片、实时过程、插话、停止、压缩、上下文引用、定点或末尾回答分叉、重命名、归档／恢复、永久删除及已读状态。工作区提供 Monaco 编辑器、版本保护保存与撤销、Run Diff、PTY 终端。
- Agent 支持命令／持续进程、补丁、MCP、Skills、Hooks 和两级子任务；每轮读取工作区根目录 `AGENTS.md`。权限有四档，人工／LLM／Jev 审批；Linux 使用 bwrap + seccomp，macOS 使用 Seatbelt。
- 模型目录在开发时由 models.dev 生成，运行时使用内置快照和用户覆盖；适配 OpenAI Chat／Responses、Anthropic Messages。ChatGPT 与 xAI 有内置账号登录，其余选定供应商使用 API Key。模型能力不等于账号权限。
- 设置集中管理外观、通知、Agent、模型、Skills、审批、Hooks 与 MCP；HTTP MCP 可从本机 Web／Desktop 发起 OAuth 授权。Windows 安装包与 macOS DMG 已接入构建。

## 技术债

| 触发场景 | 影响 | 建议的最小修复 |
| --- | --- | --- |
| 某个会话的 `meta.json` 损坏，或 `settings.json` 缺失／损坏 | [会话列表](../../internal/conversations/service.go)整体读取失败；前端仅显示通用错误，其他正常会话也不可见 | 列表逐项隔离坏会话并显示可定位的会话 ID 与原因；单会话打开继续明确报错 |
| 新增或修改 JSON-RPC 方法时只改 Go 或 TS 一侧 | 手写契约可能漂移；类型检查不能证明两端一致 | 以 appserver 登记和 `clients/contracts` 为两端事实来源；后续给关键方法加自动一致性检查，避免再维护第三份清单 |
| 项目 Hook 配置被信任后，其引用的脚本内容变化 | 当前信任只覆盖配置版本，脚本变化不会要求重新确认 | 若要把脚本内容纳入信任边界，记录并核对脚本摘要；在此之前明确向用户展示宿主执行边界 |
| 前端构建出现主包或 Monaco chunk 过大的警告 | 首次加载可能变慢，尚无实测影响量 | 先量首屏加载，再仅对确实影响首屏的模块拆分；Monaco 已使用本地资源并按需加载 |

## 限制与待验收

- Windows 受限 Agent 沙箱尚未实现；Full Access 可用。macOS Seatbelt 已在 macOS 27.0 arm64 验证，Intel 与其他版本未验收；Linux 依赖宿主提供可用的 bwrap／namespace／seccomp。沙箱不隔离硬链接别名，受保护目录内部的细粒度子路径授权暂时拒绝。
- Windows 安装、重装、升级、降级拦截、运行中拦截和卸载已实测；Mac 上已安装并核对应用图标比例，DMG 覆盖升级和数据保留仍待实机验收。WebView2 缺失环境的真实下载／安装尚未验收；隔离环境已验证失败分支。分发签名、公证、发布上传和自动更新尚未实施。
- Desktop 启动与构建已验证；Windows／macOS 的完整窗口、托盘、聊天、终端与后台通知交互仍需按平台验收。模型选择、账号状态、MCP 表单、上下文引用和嵌套子任务在亮暗主题、窄屏或真实远端环境中的专项验收也未完成。
- MCP OAuth 已由用户在阿里云远程 Server 完成授权并发现工具；刷新、重启复用和其他服务待实机验收。GitHub 托管 MCP 若不支持动态客户端注册，需提供已注册客户端身份或使用访问令牌。MCP 与 Hook 在宿主或远端执行，不在 Agent 命令沙箱内。
- ChatGPT 账号已实际用于对话；xAI 请求曾到达模型端，但工具 Schema 被拒绝，不能据此认定所有工具调用可用。两种账号的权限范围、令牌刷新和 macOS 登录仍需专项验收；第三方订阅接口的稳定性无公开承诺。LLM／Jev 已有实际使用反馈，自动审核准确率未做生产校准；供应商假服务测试不能替代真实远端验收。

未完成工作见 [Desktop 总地图](../plan/desktop/Desktop%20总地图.md)、[远控](../plan/Remote-control.md)、[Site](../plan/site/方向书.md)与[插件系统](../plan/插件系统实现方向书.md)。
