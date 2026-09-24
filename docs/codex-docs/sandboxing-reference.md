# Codex 沙箱资料入口

外部资料会变化；以下是研究入口，不是 Harness 规范。使用前核对当前文档与本地源码版本。

- [官方沙箱说明](https://learn.chatgpt.com/docs/sandboxing)：权限模式、审批与平台执行限制。
- [本地研究结论](permissions-and-sandbox-study.md)：规则、审批、Linux 隔离与 Hook 的分工。
- [Linux 结构图](linux-sandbox-map.html)：交互式机制导读。

记住三个区别：审批决定是否同意，沙箱限制实际执行；禁用审批不等于完全访问；本机命令沙箱不自动覆盖 MCP 或 Hook 的宿主执行。
