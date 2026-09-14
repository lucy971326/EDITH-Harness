# 内置编辑器与基础工具升级方向

目标：在右侧工作区提供代码浏览、小幅编辑、AI 修改审核与安全撤销。文件后端与内置编辑器已完成，当前进入 Tool 升级与 Diff。

## 原则

- 优先借鉴 `reference/codex` 已验证且适合的实现，按 Harness 现有架构适配，避免新增无必要的抽象。
- Tool 与编辑器 RPC 各有入口和规则，共享 `kernel/machine` 底层契约，由 `plugins/machine/local` 实现；文件调用与通知统一走现有 WebSocket / JSON-RPC。
- 编辑器保持克制：允许修改已有文件，不提供新建、删除、重命名；自动同步，不设手动刷新按钮。

## 阶段三：Codex 式 Tool 与 Turn Diff

- 核心心智模型：平台能力在下，Tool 只是 Agent 使用它的适配器。类比 Subagents 服务管理子会话与运行、相关 Tool 只提供入口；machine 管理文件和长期进程，文件与终端 Tool 同样保持轻薄。
- 保持 Host、machine 与 Tool 登记处边界不变；实现 `exec_command`、`write_stdin`、`apply_patch`，验证后移除旧 `read/write/edit/bash`。长期进程由 machine 管理，Tool 只作 Agent 入口。
- 翻译 Codex 的进程登记、持续输出、输入轮询、取消与清理逻辑；PTY 作为 machine 的系统适配层，不自行重写 Windows ConPTY。
- 各平台统一使用 Bash；Windows 启动时要求 Git Bash。选择 Go PTY 库后，以 Git Bash 的输入、持续输出、`Ctrl+C`、缩放和退出回收作为验收范围。
- 只记录结构化文件 Tool 已实际写入的变化，不追踪 Shell 修改。文件变化随 Tool 结果写入 Session 账本；当前 Run 在内存中聚合并缓存每文件 Diff，每次变化后推送完整 Turn Diff。
- 聊天按 Turn 展示增删行与文件列表，点击后由编辑器打开 Diff。首版支持修改后审查和按文件安全撤销；撤销前校验文件版本，避免覆盖后续修改。
- 暂不实现多环境、写入前审批、流式补丁预览和文件移动链。

### 阶段三 → A：命令与长期进程

- 实现 `exec_command` 与 `write_stdin`，由 machine 管理进程、输出、输入、取消和清理。
- `write_stdin` 沿用 Codex 参数设计，但用 `process_id` 表示进程，避免与 Harness 会话的 `session_id` 混淆；其余参数为 `chars`、`yield_time_ms`、`max_output_tokens`。
- PTY 使用 `github.com/charmbracelet/x/xpty`；Windows 使用 Git Bash。验收持续输出、交互输入、`Ctrl+C` 和退出回收；缩放等终端 UI 出现调用者后再补。

### 阶段三 → B：统一文件修改

- 实现 Codex 风格 `apply_patch`，支持多文件、多片段和上下文校验。
- 验证三个新 Tool 后，移除旧 `read/write/edit/bash`。

### 阶段三 → C：Turn Diff 与安全撤销

- 结构化文件变化进入 Session；当前 Run 按文件聚合、缓存并实时推送完整 Turn Diff。
- 聊天展示增删统计，编辑器打开 Diff；支持带版本校验的按文件撤销。

---

按阶段推进；完成事实记入 `STATUS.md`，稳定架构结论写入设计书，本计划随阶段完成收口。
