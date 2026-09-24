# Codex 研究索引

这里存放 Codex 源码笔记、外部资料摘录和相关产品对照。它们记录查阅时的机制，不是 Harness 规范；Harness 当前事实见 [STATUS.md](../../STATUS.md)，稳定决策见 [设计书](../设计书.md)。

| 类别 | 文档 | 内容 |
| --- | --- | --- |
| 源码研究 | [权限与沙箱](permissions-and-sandbox-study.md) | Codex 规则、审批与执行边界 |
| 源码研究 | [Hooks](hooks.md) | Codex Hook 事件和审批接入点 |
| 源码研究 | [Client 通信](client-communication.md) | app-server 与多端协作 |
| 源码研究 | [服务器机制](mcp-server-study.md) | exec-server、MCP Server 与 app-server |
| 资料入口 | [Sandboxing](sandboxing-reference.md) | 官方入口与关键区别 |
| 交互讲解 | [Linux 沙箱结构图](linux-sandbox-map.html) | bwrap、namespace、cgroup 与 seccomp 的分工 |
| 横向对照 | [各 Agent Harness 的 Hooks](agent-harness-hooks-survey.md) | Claude Code、Codex、Antigravity 与 Grok Build |
