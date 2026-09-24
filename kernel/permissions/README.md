# permissions

纯权限规则：计算能做什么，实际限制由 machine 实现。

```text
模式 + 工作区 + 临时目录 -> BuildPolicy
额外申请 + 基础权限     -> Evaluate -> Allow / Ask / Deny
批准                   -> ApplyDecision -> 本次权限副本
```

- `types.go`：Mode、Policy 和人／模型共用的 Reviewer 契约。
- `rules.go`：默认模式、权限比较与批准合并。
- `instructions.go`：给模型看的权限说明；说明本身不产生授权。

只读默认禁写禁网；请求批准与智能审批默认工作区、临时目录可写且禁网，区别是审核者；完全访问不添加限制。可写根内的 `.git / .agents / .harness` 仍受保护，需明确授权内部路径。

调用方提供规范化绝对路径；本包不读文件、不处理符号链接、不猜 Shell 副作用。批准不修改会话模式，审批等待属于 approvals。
