# 阶段三 → A：命令与长期进程

> ! 完成后不删除

状态：实现完成，等待阶段 B 统一替换旧 Tool。
## 概要

实现 Codex 风格的 `exec_command` 与 `write_stdin`。进程由 machine 持有，Tool 只负责参数转换和结果展示。

采用已在当前 Windows + Git Bash 实测通过的 [`xpty v0.1.4`](https://pkg.go.dev/github.com/charmbracelet/x/xpty)。local 直接使用该库，但不把其类型暴露给公共契约；没有第二个实现前不自建 PTY 适配接口。`go-pty v0.2.3` 的早期崩溃来自测试代码重复关闭 PTY，并非已确认的库缺陷；本阶段仍按已拍板方案使用 xpty。

## 核心实现

### 1. machine 进程契约

在 `kernel/machine` 增加 `ProcessSystem`，不污染编辑器使用的 `FileSystem`：

```go
type ProcessSystem interface {
    Exec(context.Context, ProcessRequest) (ProcessOutput, error)
    Interact(context.Context, ProcessInteraction) (ProcessOutput, error)
}
```

请求与结果包含：

- `ProcessRequest`：不透明 `OwnerID`、工作目录、argv、TTY、等待时间。
- `ProcessInteraction`：Owner、进程 ID、输入字节、等待时间。
- `ProcessOutput`：进程 ID、本次新增输出、是否退出、退出码和省略字节数；退出码只在已退出时有效。
- `max_output_tokens` 与调用耗时属于 Tool 呈现，不进入 machine；原始字节数由输出长度与省略字节数得出。
- 暂不导出一组无人分支处理的稳定错误；各失败返回清晰错误，并保证越权访问与不存在使用相同结果。

Tool 使用 `call.SessionID` 作为 Owner；不同会话操作同一 ID时统一返回“进程不存在”，不泄露其他会话状态。

阶段 A 的公共契约只有 `Exec` 与 `Interact`。取消、关闭和进程树终止保留为 local 包内能力；`Resize` 与显式 `Terminate` 等终端 UI 出现真实调用者后再加入契约。

### 2. 本机进程管理器

在 `plugins/machine/local` 内实现，不新增子包：

- 在 JSON 可安全表示的整数范围内生成大空间随机进程 ID，启动前预留，失败后释放，降低 Harness 重启后旧账本 ID误命中新进程的概率。
- 最多保存 64 个进程；优先清理已退出的旧记录，全是活进程时拒绝继续启动，不静默杀进程。
- local 使用一把生命周期锁保护 `closed`、监听集合与进程表，文件路径锁继续独立；避免同一关闭状态受不同锁保护。
- 每个进程保存 Owner、命令句柄、可选 PTY、退出状态、输出缓冲、状态锁、交互锁和完成信号；ID已作为进程表 Key、TTY 可由 PTY 是否存在判断，不重复存字段。
- 同一进程的输入与输出排空串行；不同进程并行。
- 输出缓冲最多 1 MiB，采用头尾保留并记录省略字节；读取线程不能因 Agent 未轮询而阻塞子进程。
- 进程退出后保留尾部输出，下一次 `write_stdin` 返回退出码并移除记录。
- machine 关闭时拒绝新进程，终止全部进程树，关闭 PTY，并等待输出读取与 Wait goroutine 收尾。

平台处理：

- `tty:false` 使用 `os/exec` 管道。
- `tty:true` 在 local 内直接使用 xpty；默认尺寸 `80×24`，不增加自己的 PTY 接口层。
- Windows 使用 Job Object 管理进程树；Unix 使用独立进程组。
- Windows TTY 执行 `bash -lic <cmd>`，使 Git Bash 能正确响应写入的 `Ctrl+C`；普通命令执行 `bash -lc <cmd>`。
- TTY 中 `\u0003` 原样写入；非 TTY 中断在 Unix 发送 `SIGINT`，Windows 终止 Job。
- 保留当前一次性 `Machine.Run` 给旧 `bash` Tool，阶段 B 再统一删除。

### 3. 两个 Agent Tool

新增一个 exec Tool 插件，同时登记：

**`exec_command`**

- 参数：`cmd`、`workdir?`、`tty?`、`yield_time_ms?`、`max_output_tokens?`。
- 工作目录默认当前工作区；相对路径从工作区解析，继续允许绝对路径和 `..`。
- 默认等待 10 秒；Windows有效范围 `10–30 秒`，其他平台 `250 毫秒–30 秒`。
- 命令提前结束则立即返回退出码；仍在运行则返回 `process_id`。
- `tty` 默认 `false`，只有交互命令显式开启。

**`write_stdin`**

- 参数：`process_id`、`chars?`、`yield_time_ms?`、`max_output_tokens?`。除将 Codex 的 `session_id` 改名以避免与 Harness 会话混淆外，其余参数保持一致。
- 空 `chars` 只轮询新增输出，默认等待 5 秒，范围 `5–300 秒`。
- 非空输入默认等待 250 毫秒，范围 `250 毫秒–30 秒`。
- TTY 可发送任意输入；非 TTY 只接受空轮询或单独的 `\u0003` 中断。

两者默认输出预算 10,000 tokens，由 Tool 在 machine 返回字节后截断并组织文本，沿用 Codex 的结果结构：

```text
Wall time: 0.1234 seconds
Process exited with code 0
```

或：

```text
Wall time: 10 seconds
Process running with process ID 1234
```

随后展示 `Original token count` 和 `Output`。非零退出码是正常命令结果；启动、查找进程或通信失败才标记为 Tool 错误。Codex 对应行为参考 [shell_spec.rs](C:/Users/Administrator/Desktop/Projects/EDITH-Harness/reference/codex/codex-rs/core/src/tools/handlers/shell_spec.rs:15) 与 [process_manager.rs](C:/Users/Administrator/Desktop/Projects/EDITH-Harness/reference/codex/codex-rs/core/src/unified_exec/process_manager.rs:813)。

## 生命周期规则

- `exec_command` 已返回 ID：进程跨 Turn 存活，用户停止 Run 也不终止它。
- 尚未返回 ID时用户停止：立即终止并回收，避免无从操作的孤儿进程。
- 后续 Turn 可从账本中的 Tool 结果取得 ID并调用 `write_stdin`。
- 进程自然退出、显式中断、容量清理或 Harness 关闭时回收。
- 进程状态只在内存中存在，Harness 重启后旧 ID返回“不存在”。
- 阶段 A 不增加 appserver 进程 RPC、前端契约或右侧终端 UI。
- 旧 `bash/read/write/edit` 暂时保留；新 Tool 出现在 Agent 设置中，现有 Agent 的选择不被后台自动改写。

## 测试与验收

- 短命令：工作目录、stdout/stderr、零及非零退出码正确。
- 长命令：到期返回 ID；轮询只返回新增输出；退出后返回尾部输出和退出码。
- TTY：Git Bash 接收输入、`\n` 与 `Ctrl+C`；Windows 实机退出码为 130。
- 生命周期：已交付 ID 的进程跨 Turn/Stop 存活；交付前取消会终止。
- 隔离：其他 Session 无法写入、轮询、缩放或终止该进程。
- 容量与输出：1 MiB 缓冲不阻塞进程，头尾与省略统计正确；达到 64 个活进程时明确拒绝。
- 关闭：Harness 关闭能够返回，且无残留 Bash 或子进程；不使用脆弱的全局 goroutine 数量断言。
- 测试保持集中且克制；实现期间只跑相关包，完成后统一执行一次 `make agent-check`。
- 完成后把事实写入 `STATUS.md`，稳定进程边界写入设计书，并从执行计划中收口阶段 A。





