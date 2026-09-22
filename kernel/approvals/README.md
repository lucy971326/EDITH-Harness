# approvals

独立审批服务，挂在 Host 的 `approvals` 键。`permissions` 负责纯规则，Tool 提供原操作，machine 强制执行最终权限。

- `Authorize`：先比较权限；需要时调用人或模型审核，生成本次 Policy 副本。
- `Settings / SaveSettings`：读取、保存全局审核方式与模型；每个申请使用开始时快照。
- `Subscribe`：原子交付待审批快照及后续完整列表；容量 1，只保留最新状态。
- `Respond`：取走并回答请求；重复、过期或取消后的回答返回冲突。

每个申请保存可信 Session / Run / ToolCall 身份和不可由客户端改写的操作。等待沿用 Run Context，不创建后台执行队列。取消或关闭收尾，断线仅解除订阅，后台重启不续接申请。待审批不写对话账本，也不保存永久授权。

本机网页订阅全部请求，含其他会话及子 Agent，并显示来源；现有本机单用户信任边界不变。人和模型实现 `permissions.Reviewer`；模型内部可返回转人工，最终仍只返回批准/拒绝。这里不执行命令，也不审核 MCP 或 Hook。

## 智能审核

同包分文件：`settings.go` 管配置，`context.go` 取用户授权，`review.go` 调常规 LLM，`jev.go` 调 TypeSafe。常规 LLM 复用模型服务与已配置模型；Jev 固定 `jev-1.13.0`，批准置信度低于 0.8 转人工。门槛是保守策略，尚未完成生产场景校准。

输入包含当前分支真实用户原话、操作、工作目录、已有权限与本次申请。委派文字不是用户授权，按来源 Session / Run 追溯；来源缺失、旧记录、图片、超预算或调用失败均转人工，不截断安全限制。模型审核总时限 45 秒，Jev HTTP 时限 30 秒；人工等待直到回答或停止。Jev 原因由代码按分类生成，未伪装为模型推理。

全局选择经 persist 保存到 `~/.harness/approvals/settings.json`。密钥与 LLM 共用 `~/.harness/config.yaml`，在现有 `providers` 旁增加：

```yaml
jev:
  apiKey: "你的 TypeSafe Key"
```

修改密钥后重启。设置页选择审核方式、模型与思考档位并保存，再将会话模式切为“智能审批”。密钥不通过设置 RPC 返回前端。
