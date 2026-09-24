# machine-local

拥有本机文件、监听、长期进程和用户终端资源，入口创建后负责调用 Close。

```text
Agent 工具 -> AgentExec / AgentApplyChanges -> 权限启动方案
用户界面   -> 文件 / 搜索 / 监听 / PTY      -> 直接操作本机
                            |
                         Local.Close -> 终止并等待
```

## 阅读顺序

- `local.go`：服务组成、Bash 定位和资源关闭。
- `files.go / search.go / watch.go`：版本保护、路径搜索、文件监听。
- `process.go / process_*.go`：进程表、PTY、增量输出与进程树清理。
- `agent.go / sandbox_*.go`：可信 Policy 转成平台启动限制。
- `agent_files.go`：内部文件助手与批量修改；助手在入口最先分流。

Agent 文件助手与编辑器共用路径锁，等待可取消；写入仍检查预读版本，中途失败返回已提交前缀。已有进程保持启动权限，进程状态不复制到工具层。

## 平台边界

- Linux：`/usr/bin/bwrap` + namespace + seccomp，需系统允许 user namespace，支持 amd64 / arm64；缺依赖或策略无法安全表达时报错。
- macOS：`/usr/bin/sandbox-exec` + Seatbelt；路径参数传入，不生成临时配置；禁止替换授权根和祖先、拒绝不安全的符号链接可写根。
- Windows：受限执行尚未实现；完全访问直接执行。用户终端使用 Git Bash。

受限写入保护 `.git / .agents / .harness`；Linux 禁网也阻断宿主 Unix socket。沙箱不承诺隔离硬链接别名或宿主同权限恶意进程。Linux 缺失保护目录的占位按引用和文件身份清理，仅删除本次创建且仍为空的目录。

不决定工具开放或审批，不把 MCP 自动放入沙箱，也不限制用户编辑器／终端。验证状态见 [STATUS](../../../STATUS.md)。
