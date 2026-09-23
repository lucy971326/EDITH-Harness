# Agent 权限系统：后续方向

权限规则、Linux / macOS Agent 沙箱、人工与智能审批、MCP 权限边界已经实施。完成事实与验证限制见 [STATUS.md](../../STATUS.md)；稳定规则见 [设计书](../../docs/设计书.md)，数据归属见 [DATA_MODEL.md](../../DATA_MODEL.md)。本目录只记录尚未实施的方向，不重复维护完成清单。

## 按需加入 Hook

目前没有 Hook 实现。只有出现明确的自定义检查需求时，才设计以下三个入口及其信任管理：

```text
PreToolUse         工具执行前的自定义检查
PermissionRequest 需要审批时的自定义决定
PostToolUse        工具执行后的自定义处理
```

`PermissionRequest` Hook 可以对**当前请求**作出决定；未作决定时交给内置审批。未配置 Hook 时，内置审批照常工作。Hook 不替代现有工具可用性、权限判断和执行沙箱。

实施前需明确配置来源与作用域、项目 Hook 的信任方式、执行与取消边界，以及 Hook 失败、修改工具参数时如何重新校验。确认这些约束后再确定接口和接入位置，不预建 Hook 登记处或配置层。
