# approvals

独立审批服务，挂在 Host 的 `approvals` 键。`permissions` 负责纯规则，Tool 提供原操作，machine 强制执行最终权限。

- `Authorize`：先比较权限；需要时调用人审核，等待并生成本次 Policy 副本。
- `Subscribe`：原子交付待审批快照及后续完整列表；容量 1，只保留最新状态。
- `Respond`：取走并回答请求；重复、过期或取消后的回答返回冲突。

每个申请保存可信 Session / Run / ToolCall 身份和不可由客户端改写的操作。等待沿用 Run Context，不创建 worker、不设自动批准超时。取消或关闭收尾，断线仅解除订阅，后台重启不续接申请。不写对话账本，也不保存永久授权。

本机网页订阅全部请求，含其他会话及子 Agent，并显示来源；现有本机单用户信任边界不变。人审核实现 `permissions.Reviewer`，模型审核尚未接入，相关额外申请明确失败。这里不做 Shell 风险分类、MCP 审核或 Hook。
