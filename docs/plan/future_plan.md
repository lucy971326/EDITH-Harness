# 后续计划

这里只记录尚未完成的工作。已完成事实见仓库根 `STATUS.md`，稳定架构见 `docs/设计书.md`。

## v1 之前

按依赖与价值排序；每一步先调研并拍板产品行为，不把后续方向提前变成空目录或抽象。

### 1. 收尾 Chat 已有能力

- 将 Chat 的 demo sidepanel 替换为真实工作区文件树与文件查看面板。

### 2. 扩展 Agent 可用输入

- 调研并拍板 MCP 如何成为 Tool / Skill 的来源，再实现；不为 MCP 改动 Runner 或 Session 的边界。当前文件系统 Skill Provider 已完成，MCP 仍未实现。
- 在真实输入需求出现后，扩展 `composer.actions` 支持多模态输入；先定义输入的落盘归属和模型适配范围。

### 3. 多 Agent 与可追溯性

- 实现 Subagent：它拥有自己的运行与业务状态，不写入父 Session。
- Subagent、MCP 已产生足够真实事实后，再实现轨迹页；它用于检查 Agent、Tool Schema、调用与结果是否符合预期。

### 4. 完整验收

- 验证产品切换、SSE 重连、慢客户端、取消和关闭。
- 完成与改动匹配的 Go、race、vet、跨平台编译与真实浏览器验收。
- 验收后同步 `docs/设计书.md` 与 `STATUS.md`。

## v1 之后再评估

### Go SDK 与插件开发 Skill

Harness 天然具备 SDK 边界：Host + Plugin 是组装入口，契约是普通 Go 接口，
kernel 不 import plugins，定义者不 import 填充者。静态编译的第三方插件不违反
“无动态插件”铁律。

正式发布 SDK 前还需要：

- 标明承诺稳定的公共 API 与内部实现。
- 为服务键提供稳定常量和清晰的 Resolve 错误。
- 以 Go v0 版本发布，并认真对待契约破坏。
- 写一个仓库外插件，真实验证 import、组装和文档边界。
- 从 `AGENTS.md` 蒸馏插件开发 Skill。

Skill 示例使用：

```text
plugins/kernel/tools/bash          最小内核插件
plugins/web/chat/dock/demo         Chat 插槽填充物
```

时机：

```text
现在          继续完成 v1，不冻结尚未充分使用的 API
v1 之后       发布 Go v0，用仓库外插件验证
出现外部用户   再发布插件开发 Skill
```

明确不做动态插件和面向最终用户的插件市场。MCP 继续使用自己的进程配置；
开发者直接使用 Go 模块、契约和 Skill 编写静态插件。

### 其他产品方向

- 电影、狼人杀与更多面板只在真实需求出现时实现。
- 不为了展示未来方向提前创建空目录或抽象接口。

### 持久状态的边界

“所有产品统一 Store”不是已排期的功能。等第一个真实产品需要独立持久状态时，再根据它的数据形状决定：优先由该插件定义自己的数据与 Store 契约，并复用 `persist` 的持久化基础；不预先创建万能键值抽屉。
