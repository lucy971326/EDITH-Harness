# 第二步：Agent 执行通道与 Linux 沙箱

本阶段实施记录收口到 [STATUS.md](../../STATUS.md)，稳定边界见 [设计书](../../docs/设计书.md)，调用入口见 [machine README](../../kernel/machine/README.md)。

```text
Agent 命令 / 补丁 → 可信 Policy → Agent 专用入口
                                  ↓
                           bwrap + seccomp
                                  ↓
                             本机文件与进程
用户编辑器 / 终端 ────────────────↗
```

下一步按 [overview](overview.md) 接独立审批服务：批准后生成本次权限，再进入 Agent 通道。后续新增细粒度授权时，先补齐对应挂载表达与验证；不能把尚不支持的授权静默扩大。
