# Linux 权限与沙箱 · 学习计划

## 目标

为 Harness 的 Linux / WSL2 Agent 执行边界补齐必要知识，最终理解并验证：

```text
权限策略 Allow / Ask / Deny
          ↓
bubblewrap：进程、文件与网络隔离
          ↓
seccomp：缩小系统调用面
          ↓
cgroup + 信号 + wait：限制资源并完整收尾
```

重点是知道每层保护什么、删除后会坏什么；不学习完整内核，也不自己重写 bubblewrap。

## 学习对象

Docker、Podman 和 bubblewrap 都是在组合 Linux 内核提供的基础机制：

```text
Docker / Podman / bubblewrap
        ↓ 负责组合与管理
┌────────────────────────────┐
│ namespace    隔离资源视图   │
│ cgroup       限制资源使用   │
│ capabilities 拆分 root 权力 │
│ seccomp      过滤系统调用   │
│ LSM          强制访问控制   │
│ mount        构造文件系统视图│
│ UID/GID      进程与文件身份 │
└────────────────────────────┘
        ↓
     Linux 内核
```

本计划学习这些底层机制；Docker 用于反向验证理解，Harness 最终按需要组合 bubblewrap、seccomp 与进程监管。

## 一、进程与继承 ✅ 已学习

- [x] PID、PPID、进程树与孤儿进程
- [x] `fork`、`exec` 与权限继承
- [x] 进程组、Session、信号、退出码与 `wait`
- [x] 文件描述符和环境变量的继承

动手实验：`go run .forward/linux/process_group.go`，先观察一个子进程的新 Session、进程组信号与 `Wait`。

**验收：**能解释超时、父进程退出和后台派生时，如何停止并回收整棵进程树。

## 二、身份与文件权限 ✅ 已学习

- [x] UID、GID 与补充组
- 文件和目录的 `rwx`、`umask`、sticky、setuid / setgid
- inode、硬链接、符号链接与路径逐段解析
- ACL、capabilities 与 `no_new_privs`（已接触，后续按需回顾）

**验收：**能解释常见读写删除实验、链接越界风险，以及为什么路径字符串检查不能构成沙箱。

## 三、隔离与资源限制 ✅ 已学习

- user / mount / PID / network namespace：分别隔离身份、文件视图、进程和网络
- cgroup v2 与 rlimit：限制 CPU、内存、进程数和文件描述符
- seccomp：过滤系统调用，但不冒充完整沙箱
- Landlock、AppArmor / SELinux：了解与 namespace、seccomp 的职责差异

**验收：**能为每种机制各写一句“保护什么”和“不保护什么”。

## 四、bubblewrap 实验

- 用普通用户建立最小沙箱，只暴露必要程序、库、设备和临时目录
- 分别验证工作区只读、工作区可写、工作区外不可见和禁用网络
- 清理不需要继承的环境变量、文件描述符和 capabilities
- 叠加 `no_new_privs`、seccomp、cgroup、超时与进程树回收
- 用 `/proc`、`strace`、`namei`、`capsh` 观察真实边界

**验收：**用一张测试表证明允许的行为正常，文件越界、网络、提权、资源耗尽和残留进程受到预期限制。

## 五、映射回 Harness

- `machine-local` 原始机器通道与 Agent 受控通道在哪里分开？
- Read Only、Ask for approval 与 Full Access 各自使用什么沙箱边界？
- 为什么审批只能允许一次操作，不能扩大整轮 Sandbox？
- Tool、MCP 与子 Agent 如何共用一条权限和执行主干？
- 启动失败、Stop 或 Harness 关闭时，怎样拒绝执行并回收全部进程？
- 哪些保证来自 Harness 策略，哪些必须由 Linux 内核强制执行？

**最终验收：**能画出一次 Agent 操作从权限决策到沙箱执行和进程收尾的完整链路，并指出删除每层后的具体后果。

## 暂不深入

内核源码、调度算法、驱动、磁盘结构、Kubernetes、自制 namespace 启动器、自制 seccomp BPF 编译器及复杂 SELinux / AppArmor 策略；遇到真实实现阻塞时再补。

## 顺序

```text
进程继承 → 身份与文件权限 → 隔离与资源 → bubblewrap 实验 → Harness 设计
```
