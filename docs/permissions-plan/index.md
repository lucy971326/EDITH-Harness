# 权限系统方向书

## 目标

为 Agent 建立可理解、可审批、可强制执行的权限边界，让用户能在安全性与自主性之间明确选择。

本方向参考 Codex 的成熟设计，只保留四种内置模式，不提供 Custom 模式。

## 产品形态

| 模式 | 行为 |
| --- | --- |
| Read Only | 默认只读，越界操作询问用户 |
| Ask for approval | 可在工作区内行动，越界时询问用户 |
| Approve for me | 权限范围不变，由独立审查 Agent 处理可自动审查的请求 |
| Full Access | 不受 Agent 沙箱限制，也不发起审批 |

## 心智模型

```text
Tool 是否可用
      ↓
Hook：检查、阻止或补充规则
      ↓
权限策略：Allow / Ask / Deny
      ↓ Ask
用户审批或自动审查
      ↓ Allow
OS Sandbox 内执行
```

- **Tool 权限**决定 Agent 能否调用某个工具。
- **权限策略**决定本次操作能否执行、是否需要审批。
- **审批**表达用户或审查 Agent 是否同意一次越界。
- **OS Sandbox**是最终边界，即使工具中的代码不可信也不能越界。
- **Hook**是策略扩展点，不代替权限判断和操作系统隔离。

## 架构方向

- 权限模式属于会话运行设置；一轮执行始终使用明确的有效权限。
- 所有 Agent Tool、MCP 和子 Agent 操作统一经过权限决策链，不建立旁路。
- 待审批请求属于活跃运行，不进入对话账本；重新连接后仍能看到并处理尚未结束的请求。
- 自动审查只改变“谁来审批”，不扩大 Sandbox、文件、网络或进程权限。
- `machine-local` 提供 Agent 专用的受控执行通道，同时保留用户编辑器和终端使用的原始机器通道。
- Agent 执行最终由平台 Sandbox 强制限制，不能依赖命令字符串识别来保证安全。

## 平台方向

```text
macOS       → Seatbelt
Linux/WSL2  → bubblewrap + seccomp
Windows     → 参考 Codex 原生 elevated / unelevated 沙箱
```

Windows 不从零设计低层隔离；优先复用或借鉴 Codex 已验证的用户、ACL、网络、Restricted Token 与进程管理边界。

## 明确边界

- 不提供 Custom 权限模式。
- 不把审批状态写入 Session 账本。
- 不用 Hook 或命令分类冒充 Sandbox。
- 不全局限制 `machine-local`，避免影响用户主动使用编辑器和终端。
- 不为单个 Tool 各造一套审批机制。
- 不因实现权限系统而改变 Runner、Loop、Session 的既有职责。

## 成功形态

用户能看懂当前模式；所有 Agent 副作用都经过同一权限主干；需要审批时可以由用户或自动审查处理；即使误判或工具代码不可信，平台 Sandbox 仍能守住最终边界。
