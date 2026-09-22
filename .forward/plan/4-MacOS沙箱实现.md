# 第 4 阶段：macOS 沙箱

## 难度判断

**中等，现有结构已经很适合接入。** 权限规则、审批、命令执行、文件修改助手都能复用，主要补上 Mac 的平台实现。

已用 CodeGraph 定位并核对 Codex 源码：它生成 **Seatbelt 规则文本**，通过 `/usr/bin/sandbox-exec -p` 启动受限命令，路径通过 `-D` 参数传入，无需临时配置文件。

真正需要仔细处理的是：**路径别名、保护目录、终端与联网兼容性。** Linux 上可以编译检查，实际限制是否正确由 Mac 实机验证。

## 要做什么

```text
模式 + 工作目录 + 本次批准
             │
             ▼
         有效权限 Policy       ← 已有
             │
      ┌──────┴──────┐
      ▼             ▼
 Linux 翻译器    Mac 翻译器      ← 本次新增
 bwrap/seccomp   Seatbelt 规则
      │             │
      └──────┬──────┘
             ▼
     命令 / 文件修改助手
```

- 在 `plugins/machine/local` 增加 `sandbox_darwin.go`，实现已有的 `prepareSandbox`；其他平台兜底改为排除 Darwin。
- 保留现有公共接口与 `agentLaunch`，不增加插件、权限字段或前端设置。
- 基础规则参考 Codex：默认拒绝，开放读取、必要的进程运行与终端能力；仅允许授权目录写入，联网按 `Policy.Network` 开关控制。
- 策略保存在内存，用独立命令参数传给系统固定路径 `/usr/bin/sandbox-exec`。工具缺失、规则无效或启动失败就报错，不自动转为完全访问。

## 必须守住的边界

- `/tmp`、`/var` 等系统顶层路径别名转换为实际路径；授权目录须存在，拒绝内部经过符号链接的可写根。
- `.git`、`.agents`、`.harness` 延续当前保护规则，兼顾路径本身与子目录；保护路径即使尚不存在也不能绕过。
- 防止删除或替换授权根及保护目录的祖先来绕过规则；参考 Codex，禁止可绕过普通写入检查的特殊 `fcntl` 操作。
- 首版沿用 Linux 对保护目录内部细粒度授权的限制，无法可靠表达的策略明确拒绝。
- 禁网同时限制 IP 网络和宿主 Unix socket；联网时补齐 DNS、证书服务所需规则。
- 子进程继承沙箱，已有进程沿用启动权限；完全访问仍走原有直接执行入口。

## 极简伪代码

```go
// 已有流程
prepareAgentLaunch(policy, request):
    if policy.Unrestricted:
        return 直接启动(request)

    return prepareSandbox(policy, request)

// 新增 Mac 实现
prepareSandbox(policy, request):
    检查系统 sandbox-exec
    校验授权路径与保护范围

    规则 = 基础规则 + 读取规则
    规则 += 授权目录写入规则
    规则 += 保护目录与防绕过规则

    if policy.Network:
        规则 += 联网规则

    return 启动方案(
        "/usr/bin/sandbox-exec",
        "-p", 规则,
        路径参数,
        "--", 原命令,
    )
```

## 验证与交付

只新增一组 Mac 核心集成测试，覆盖：

- 项目内写入成功，项目外与保护目录写入失败；包含新建、删除和符号链接绕行。
- 只读拒绝写入，本次批准后成功，下一次恢复基础限制。
- 禁网拒绝本机 TCP／Unix socket，批准联网后成功。
- 文件修改助手、PTY 交互及停止进程正常。

实现完成执行一次 `make agent-check`，并交叉编译 Mac arm64、amd64。随后你在 Mac 上运行核心测试和 `make run`，验证真实 HTTPS 与审批流程；未完成实机验证前，不标记 Mac 沙箱已验收。

同步包 README 与 `STATUS.md`。本阶段不接入智能审批、Hooks 或 Windows 沙箱。

## 在 Mac 上怎么测试

### 1. 同步代码，运行核心测试

在 Mac 的项目根目录执行；需要已安装 Go、Node.js 与 npm。先确认系统启动器存在：

```bash
ls -l /usr/bin/sandbox-exec
go test ./plugins/machine/local -run TestDarwinAgentSandbox -v -count=1
```

`-count=1` 强制实际执行，不使用测试缓存。预期最后显示 `PASS`；这组测试不需要模型配置、不访问外网，探针文件在测试临时目录内。若显示没有测试可运行，确认正在 Mac 上、且已同步本次代码。

失败时保留完整输出，并附上 `sw_vers` 和 `uname -m` 的结果，便于定位系统版本或 CPU 差异。

### 2. 启动网页

Mac 需要单独配置本机 `~/.harness/config.yaml` 中的模型 Provider 与 API Key；这份文件不随 Git 同步。然后执行：

```bash
make run
```

选择 Mac 上真实存在的测试项目目录，不沿用 Linux 的 `/home/lucy/...` 路径。

### 3. 验证审批与真实执行

使用临时测试项目，选择「请求批准」，分别让 Agent 做以下操作：

| 操作 | 预期 |
| --- | --- |
| 在项目内创建、修改、删除一个测试文件 | 直接成功；同时试一次 `apply_patch` |
| 不申请额外权限，写入项目外的专用测试目录 | 被拒绝 |
| 申请该测试目录的写入权限，点击「允许一次」 | 本次成功；下一条不申请仍被拒绝 |
| 申请联网，执行下方 HTTPS 命令 | 先出现审批；批准后输出 HTTP 状态码 |
| 再执行同一联网命令，但不申请额外权限 | 被拒绝联网 |
| 发起审批后点击「拒绝」，或停止任务 | 不执行申请的操作；输入框恢复 |

联网测试命令：

```bash
curl -sS -m 10 -o /dev/null -w 'HTTP %{http_code}\n' https://example.com
```

再切到「只读」验证项目内写入也被拒绝；切到「完全访问」验证普通命令可以运行。Agent 的 PTY 可用交互命令测试；右侧用户终端走直接通道，不用于证明 Agent 沙箱生效。

审批等待时刷新网页，应恢复同一申请。Mac 实机结果确认后，再将验收结论记入 `STATUS.md`。

## 实机验证结果

macOS 27.0 arm64 已通过核心集成测试，以及项目内外写入、单次授权、真实 HTTPS、只读、完全访问和交互式 PTY 的手工验收。单次授权不会延续，未申请的联网会被拒绝；停止与刷新审批本轮未重复验证。
