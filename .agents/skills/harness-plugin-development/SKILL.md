---
name: harness-plugin-development
description: 在 Harness 中新增或调整静态插件、服务与登记处，检查依赖归属、启动组装和资源关闭。用于插件接入或边界变更，不用于仅修改已有插件内部算法、文案或样式。
---

# 插件开发与组装

让新增行为进入正确的服务或插槽，并能随 Host 正常启动和关闭。只处理用户要求的改动，不顺带设计新平台或重构其他领域。

## 读取入口

本 Skill 位于 `.agents/skills/harness-plugin-development/`；下面的链接相对本目录，代码路径相对仓库根。

- 先读 [AGENTS.md](../../../AGENTS.md) 与 [STATUS.md](../../../STATUS.md)，本轮已读可复用。
- 选择服务或插槽时查 [设计书](../../../docs/设计书.md) 的相关领域；新增持久数据时读 [DATA_MODEL.md](../../../DATA_MODEL.md)；涉及 Web 页面或插槽时读 [WEB_UI.md](../../../WEB_UI.md)。
- 文档用于确认约束，实际签名与已实现行为以当前源码核对；发现冲突先说明，不把示例代码当成永久 API。

## 先确定归属

用项目的四问简短说明：它是什么、提供什么服务、使用什么服务、填哪个插槽。再决定代码位置：

```text
已有服务内部的一项行为 → 同包文件或函数
已有登记处的一种实现   → 填入条目
独立领域且别人需要调用 → 提供一份服务
仅供用户进入的 Web 产品 → 填 Web products / routes
```

不要因为功能新就增加 Host 服务键。先查定义者的 `types.go`、登记处和实际调用者，确认现有入口够不够用。

## 选一个最接近的实现阅读

| 本次任务 | 代码入口 | 重点 |
|---|---|---|
| 注册普通 Tool | `plugins/kernel/tools/bash/`、`kernel/tools/types.go` | 获取 machine、构造工具、填 tools |
| 接入动态 Tool 来源 | `plugins/kernel/tools/mcp/plugin.go`、`kernel/tools/types.go` | 用户选择与工作区快照、资源归属；不照搬全部 MCP 实现 |
| 提供独立服务 | `kernel/runner/plugin.go`、`kernel/host/host.go` | 构造、Resolve、RegisterService 的边界 |
| 添加 Web 产品或插槽 | `plugins/web/chat/plugin.go`、`surface/web/types.go` | 确认填的是 Web 登记处还是 Chat 自己的登记处 |

只读相关一行的实现。修改 Loop 执行语义时，再读 [Loop 开发与验收](../harness-loop-development/SKILL.md)。

## 实现时检查

- 先确定契约由谁定义，再按 AGENTS.md 的类型与 import 规则放文件。消费者通过定义者使用服务，不 import 具体提供者。
- `plugin.go` 负责接线与生命周期；业务实现放所属领域文件。组装入口是 `cmd/harness/main.go`，按现有必装 / 可选规则接入，不让配置承担排序。
- 打开的长期资源要有明确主人。检查 `Start` 中途失败、正常 `Close` 时如何释放；后台任务取消后要等待退出。Host 会关闭启动失败的插件，因此部分初始化状态也要能清理。
- 修改公共契约时搜索所有调用点；不要留下另一套转发 API 只为保留旧写法。

## 按风险验收

普通登记条目先运行受影响包的已有测试。涉及资源生命周期时，重点验证启动失败能清理、关闭后任务退出；涉及契约变更时编译或测试调用方。没有实际风险的样板接线不额外堆测试。

交付简述：填了哪个入口、谁拥有资源、验证了什么。规则变化更新规则来源，完成状态按需更新 STATUS.md；不要在 Skill 里复制一份架构规范或维护第二份进度。
