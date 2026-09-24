# Codex 权限与沙箱研究

> 历史研究记录：Codex 源码研究参考本地 `reference/codex` 提交 ce24367；本篇仅记录当时的外部实现，不代表当前 Harness 规范。Harness 现状见 [STATUS.md](../../STATUS.md)，稳定决策见 [设计书](../设计书.md)。

## 目标

让 Agent 在明确边界内自主工作；需要越界时，用户知道它想做什么、由谁批准，以及批准后实际放宽了什么。

一句话心智模型：**策略决定能否申请，审批决定谁同意，Sandbox 限制真正执行的进程。Hook 可以插入检查，但不代替 Sandbox。**

## 心智模型

    Agent 提出 Tool Call
             ↓
    Tool 是否可用？
             ↓
    PreToolUse Hook：可检查、阻止或改参数
             ↓
    权限策略：Allow / Ask / Deny
             ↓ Ask
    PermissionRequest Hook → 用户或自动审查
             ↓ Allow
    在本次有效 Sandbox 中执行
             ↓
    PostToolUse Hook：处理已经产生的结果

- **Tool 权限**：这个 Tool 是否对 Agent 开放。
- **权限策略**：这次操作直接放行、询问，还是拒绝。
- **审批**：一次越界申请由谁同意；同意并不会改变宿主机原有的用户身份。
- **OS Sandbox**：本机命令的强制执行边界。它限制文件、网络等真实能力，子进程继承边界。
- **Hook**：按事件运行的扩展程序，可加入检查或反馈；它不覆盖所有工具路径。

这条图是理解本机命令的主干。MCP／App 工具会经过各自的启用和审批规则，服务端也可能在另一台机器运行；不能把本机 shell 的 Sandbox 当成所有 Tool 的共同外壳。[工具编排](../../reference/codex/codex-rs/core/src/tools/orchestrator.rs#L141)、[MCP 入口](../../reference/codex/codex-rs/core/src/mcp_tool_call.rs#L212)

## Codex 已有：一次命令怎样通过

1. **先判规则。** 执行规则（execpolicy）按命令前缀给出 Allow、Prompt、Forbidden。未匹配时还会做危险命令判断，例如带强制参数的 rm。Forbidden 直接拒绝。
2. **再判审批。** 当前沙箱内可完成的普通命令通常直接运行；要写到边界外或联网，可以提出提权请求。on-request 可以询问，never 不弹常规审批。一次批准、会话内批准、持久命令规则的范围不同。
3. **确定审核者。** 真正需要审批时，先运行 PermissionRequest Hook；它没作决定才交给自动审核者或用户。拒绝后命令不会执行。
4. **选择执行边界。** Codex 先选沙箱再启动工具。受限执行失败，不是任何错误都会自动改成无沙箱重试；有“不可读路径”限制时，提权也不能直接丢掉文件沙箱。

注意：显式 Allow 命令规则在每一段命令都匹配时，**可能允许跳过沙箱**，不只是免弹窗。因此这类持久规则应保持较窄。[规则文档](../../reference/codex/codex-rs/execpolicy/README.md)、[规则判定](../../reference/codex/codex-rs/core/src/exec_policy.rs#L394)、[审批与重试](../../reference/codex/codex-rs/core/src/tools/orchestrator.rs#L314)、[不可读路径保护](../../reference/codex/codex-rs/core/src/tools/sandboxing.rs#L239)

### 一个具体例子

假设项目在 /home/lucy/project，另有用户有权写入的 /home/lucy/notes，当前模式是 Ask for approval：

| Agent 想做 | 通常发生什么 |
| --- | --- |
| 在项目里创建文件 | 工作区可写，直接在沙箱中执行 |
| 在 notes 里创建文件 | 超出工作区，须申请额外权限 |
| 直接连接外网 | 网络受限，须申请网络例外 |
| 执行被规则禁止的危险命令 | 直接拒绝，不能靠普通审批绕开禁令 |

批准 notes 写入，只是放宽 Codex 的这次限制；如果宿主机本来不允许 lucy 写入，批准也不会让写入成功。

## Codex 已有：Linux 真正怎样限制

常见工作区受限路径：

    Codex → bubblewrap 建文件视图与命名空间
          → 沙箱内 helper 安装需要的 seccomp 限制
          → 启动命令，子进程继承限制

| 机制 | 实际作用 |
| --- | --- |
| **bubblewrap + mount namespace** | 通常先把宿主文件树接成只读，再把项目、/tmp 等允许写的目录重新接成可写；受保护的 .git、.agents、.codex 等子路径可重新设成只读。文件仍是宿主真文件。 |
| **user / PID / IPC namespace** | 分隔身份环境、进程视图和进程间通信；通常挂新的 /proc，并丢弃 Linux capabilities。 |
| **network namespace** | 禁网或托管代理时隔开宿主网络；代理模式再接一条受控的代理桥。 |
| **seccomp** | 拦指定系统调用：禁网时限制 connect 等网络操作，相关模式还限制 ptrace、跨进程内存读写和 io_uring。它不是“所有系统调用逐个审批”。 |
| **no_new_privs** | 在需要 seccomp 等限制时设置，阻止执行程序后取得新特权；源码按条件设置。 |

两个例外让图保持准确：**只禁网、文件完全开放**时可以直接安装网络 seccomp，不启动 bubblewrap；**文件受限、普通网络允许**时仍用 bubblewrap，seccomp 主要封住 AF_VSOCK 等绕行入口。[Linux 启动流程](../../reference/codex/codex-rs/linux-sandbox/src/linux_run_main.rs#L155)、[挂载与命名空间](../../reference/codex/codex-rs/linux-sandbox/src/bwrap.rs#L353)、[seccomp 规则](../../reference/codex/codex-rs/linux-sandbox/src/landlock.rs#L175)

**只读不等于不可读。** 工作区模式一般仍可能读取宿主机上当前用户可读的文件；明确设为不可读的路径才会被遮盖。此处研究的 Linux helper 没有用 cgroup／rlimit 分配 CPU、内存或进程数配额，也没有由它配置 AppArmor／SELinux。旧 Landlock 文件沙箱代码仍在，但当前受限文件路径要求 bubblewrap，以隔离 app-server Unix socket。[Linux 沙箱说明](../../reference/codex/codex-rs/linux-sandbox/README.md)、[不可读路径](../../reference/codex/codex-rs/linux-sandbox/src/bwrap.rs#L484)

## Codex 已有：Hooks 在哪里起作用

| Hook | 能做什么 | 边界 |
| --- | --- | --- |
| **PreToolUse** | 工具执行前检查、阻止或改参数 | 它的 allow 不是提权批准，后面仍要判审批与沙箱 |
| **PermissionRequest** | 只有出现审批请求时，允许、拒绝或交给正常审批 | 不会替每条普通命令审查 |
| **PostToolUse** | 改写或阻止模型看到工具结果 | 工具已经执行，不能撤销文件修改 |

Hook 脚本出错或超时通常会记录失败并继续原来的流程；不能把它当成强制隔离层。普通 Hook 需要用户按定义信任，修改后重审；项目 Hook 仅从受信任项目加载。Hook 命令自身也不是自动套在被检查命令的 bubblewrap 中。部分托管工具不走本地 Tool Hook 路径。[工具 Hook 顺序](../../reference/codex/codex-rs/core/src/tools/registry.rs#L588)、[审批 Hook 顺序](../../reference/codex/codex-rs/core/src/tools/approvals.rs#L500)、[Hook 执行](../../reference/codex/codex-rs/hooks/src/engine/command_runner.rs#L206)、[官方 Hooks 文档](https://learn.chatgpt.com/docs/hooks)
