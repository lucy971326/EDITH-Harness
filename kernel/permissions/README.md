# permissions

【它是什么】Agent 权限计算与审核契约。纯 Go 包，不是插件，不登记 Host 服务。

【使用能力】调用方提供模式、工作区和可信临时目录；本包不读取环境变量或文件系统。

【提供能力】

- `NormalizeMode`：空模式补为 `ask_for_approval`，未知模式报错。
- `Resolve`：模式与运行位置 → 基础 `Policy`，同时选出人、模型或无需审核。
- `Evaluate`：比较额外申请与已有权限，返回 `Allow`、`Ask`；非法路径返回 `Deny` 和错误。
- `ApplyDecision`：批准后生成本次权限副本；拒绝返回错误，不修改基础权限。
- `Reviewer`：人审批与模型审批共用的接口，只返回批准或拒绝及理由。

```text
SessionSettings.PermissionMode + 工作区 + 临时目录
                         ↓ Resolve
                      基础权限
                         ↓ Evaluate（额外申请）
          ┌──────────────┼──────────────┐
        Allow           Ask           Deny
          │              ↓              │
          │       外部审批服务审核       拒绝
          │              ↓ ApplyDecision
          └──────── 本次有效权限
                         ↓
                  交给执行层落实
```

【权限规则】所有模式允许读取宿主当前用户可读文件。Read Only 默认不可写、禁网；Ask for approval 和 Approve for me 默认工作区与传入的临时目录可写、禁网，区别只在审核者；Full Access 不添加 Agent 文件与网络限制。

`Policy` 只存 `Unrestricted`、`WriteRoots`、`Network`。可写根下的 `.git`、`.agents`、`.harness` 默认受保护，明确授权其内部路径才能放开；宽泛的父目录授权不能解除保护。

【谁在用】SessionSettings 引用模式类型，persist 处理默认值与保存校验，Harness Product 校验设置。审批与执行层的接入状态以根目录 [STATUS.md](../../STATUS.md) 为准。

【调用边界】路径必须是规范化绝对路径。规则只做词法判断，符号链接与路径替换竞态由执行层处理。审批服务负责把决定绑定到原申请，失败或取消不能视为批准；本次权限不写回 SessionSettings。

【不做】不决定 Tool 是否开放，不猜测 Shell 副作用，不管理审批等待或 Hooks，不调用模型、弹窗或启动沙箱。图中的审批与执行由包外实现，计算出权限本身不代表隔离已经生效。
