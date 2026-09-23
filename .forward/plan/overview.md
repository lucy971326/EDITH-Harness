# Agent 权限系统：后续方向

权限规则、Linux / macOS Agent 沙箱、人工与智能审批、MCP 权限边界已经实施。完成事实与验证限制见 [STATUS.md](../../STATUS.md)；稳定规则见 [设计书](../../docs/设计书.md)，数据归属见 [DATA_MODEL.md](../../DATA_MODEL.md)。本目录记录尚未实施的方向及相关调研，不重复维护完成清单。

## 按需加入 Hook

其他产品的触发点与处理方式见 [Hooks 机制对照](Hooks-机制对照.md)。

`PreToolUse` 的现状见 [STATUS.md](../../STATUS.md)。后续只有出现明确需求时，再考虑以下入口：

```text
PermissionRequest 需要审批时的自定义决定
PostToolUse        工具执行后的自定义处理
```

`PermissionRequest` Hook 可以对**当前请求**作出决定；未作决定时交给内置审批。未配置 Hook 时，内置审批照常工作。后续 Hook 不替代现有工具可用性、权限判断和执行沙箱。

引入新事件前，按其能力明确输入输出、信任、失败与取消边界；若允许修改工具参数，还须重新校验。没有具体需求时不预建 Hook 登记处。
