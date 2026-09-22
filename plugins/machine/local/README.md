# machine-local

【它是什么】本机 `machine` 服务提供者；不 Resolve 其他服务。

【提供能力】文件读写、版本检查、搜索、监听、长期进程和 PTY；Agent 与用户入口明确区分，共享底层资源与清理逻辑。

```text
AgentExec / AgentApplyChanges → 启动方案 → Linux bwrap + seccomp
AgentInteract                → 已有进程（权限保持不变）
用户编辑器 / 终端            → 直接操作本机
```

【代码入口】`agent.go` 准备启动方案，`sandbox_linux.go` 构造 Linux 限制，`agent_files.go` 批量提交与内部写入助手；`process.go` 管理进程，原文件/搜索/监听代码供各入口共用。

【运行依赖】受限 Linux 执行需要 `/usr/bin/bwrap`，支持描述符挂载与 seccomp，并允许 user namespace。只支持 amd64 / arm64 的 seccomp ABI；失败不自动降级。Mac / Windows 受限执行明确报未支持，Full Access 直接执行。

【限制】根文件系统只读，可写根覆盖为可写；根下 `.git`、`.agents`、`.harness` 受保护。禁网模式阻断 TCP、UDP 与宿主 Unix socket，包括 datagram 发送绕行；流式 socketpair 保留。受限程序不能重新挂载、注入其他进程或用 io_uring 绕过过滤。复杂的保护目录内部子路径授权暂时拒绝；沙箱不是硬链接别名或宿主同权限恶意进程的完整隔离。

【临时资源】缺失保护目录可以创建空占位；共享引用，退出时只删除本次创建、身份匹配且仍为空的目录。强制杀死整个宿主或断电可能留下空占位，不自动删除身份未知的目录。

【文件助手】可执行文件在正常启动前调用 `RunFileWorker` 分流内部模式；只通过管道交换修改与提交结果，不启动 Host。父进程按固定 key 取得编辑器同一组路径锁，等待可取消；助手执行版本检查。Full Access 也使用助手，但不加沙箱；文件 I/O 不持有 machine 服务锁，助手统一登记、终止与回收。取消后不伪造精确 Diff。

【不做】不决定 Tool 是否开放，不管理审批或 Hooks，不限制用户编辑器/终端。MCP 的执行不自动进入此通道。能力与验证状态见 [STATUS.md](../../../STATUS.md)。
