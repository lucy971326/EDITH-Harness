# 后续计划

这里只记录尚未完成的工作。已完成事实见仓库根 `STATUS.md`，稳定架构见 `docs/设计书.md`。

## v1 之前

### 页面插槽的真实填充物

- 为 `message.actions` 增加 Session 分叉动作。
- 将 Chat 的 demo sidepanel 替换为真实工作区文件树与文件查看面板。

### 阶段 5：完整验收

- 实现同一 Session 的轨迹页。
- 验证 Session 分叉后的页面导航与当前分支历史。
- 验证产品切换、SSE 重连、慢客户端、取消和关闭。
- 完成 Go、race、vet、跨平台编译与真实浏览器验收。
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

- Subagent、电影、狼人杀与更多面板只在真实需求出现时实现。
- 不为了展示未来方向提前创建空目录或抽象接口。
