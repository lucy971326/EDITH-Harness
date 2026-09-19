# 权限系统学习清单

原则：围绕 Harness 边做边学，不先啃完整操作系统教材。

## 一、进程与权限

- [ ] 理解进程、父子进程、进程树与权限继承
- [ ] 理解用户、用户组、环境变量、句柄与文件描述符
- [ ] 区分 Shell、PTY 与普通管道
- [ ] 掌握信号、退出码、取消与僵尸进程
- [ ] 实验：启动多级子进程并可靠终止整棵进程树

## 二、Go 系统编程

- [ ] 掌握 `context` 取消传播
- [ ] 掌握 goroutine、channel、锁与资源生命周期
- [ ] 掌握 `os/exec`、标准输入输出与 PTY
- [ ] 理解跨平台 build tag 与系统调用封装
- [ ] 实验：实现可输入、可取消、可等待的长期进程

## 三、文件系统安全

- [ ] 理解路径规范化、相对路径与绝对路径
- [ ] 理解软链接、硬链接、挂载点与路径穿越
- [ ] 理解 ACL、原子写入与 TOCTOU
- [ ] 理解 Windows 与 Unix 文件权限差异
- [ ] 实验：验证软链接不能绕过工作区边界

## 四、安全基础

- [ ] 理解最小权限、默认拒绝与纵深防御
- [ ] 区分权限策略、审批和强制隔离
- [ ] 理解 Fail Open、Fail Closed 与信任边界
- [ ] 理解审批放行一次究竟扩大了什么权限

```text
Policy  决定应该怎样
Sandbox 保证只能怎样
```

## 五、平台 Sandbox

- [ ] Linux：namespace、bubblewrap、seccomp、capabilities、cgroup、Landlock
- [ ] macOS：Seatbelt 策略与进程继承
- [ ] Windows：Restricted Token、ACL、Job Object、WFP、AppContainer
- [ ] 实验：限制文件写入、网络访问和子进程逃逸

## 六、Agent Runtime 权限

- [ ] 理解 Tool、Hook、Policy、Reviewer、Sandbox 的执行顺序
- [ ] 理解用户审批与自动 Reviewer 的职责差异
- [ ] 理解审批期间的 Stop、取消、断线与重复回答
- [ ] 理解待审批状态为何不进入 Session 账本
- [ ] 验证 Tool、MCP 与子 Agent 不存在权限旁路

## 完成标准

- [ ] 能解释一条 Agent 命令从 Tool Call 到 OS 执行的完整链路
- [ ] 能指出策略判断与真正安全边界分别在哪里
- [ ] 能设计进程、文件、网络和审批的失败恢复
- [ ] 能判断哪些能力适合自己实现，哪些必须复用成熟方案
