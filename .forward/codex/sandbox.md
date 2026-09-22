# Sandbox

> 完整的文档索引请参阅 [llms.txt](https://learn.chatgpt.com/llms.txt)。在页面 URL 末尾添加 `.md` 即可获取文档页面的 Markdown 版本。

Sandbox 是让 agent 能够自主执行操作、同时避免其对你的机器拥有无限制访问权限的边界。当本地 chat 在 **ChatGPT desktop app**、**Codex CLI** 或 **IDE extension** 中运行命令时，这些命令默认会在一个受限环境中执行，而不是拥有完全访问权限。

该环境定义了 agent 可以独立执行的操作，例如可以修改哪些文件以及命令是否可以使用网络。当任务保持在这些边界之内时，agent 可以持续执行而无需停下来等待确认。当它需要超出这些边界时，approval 流程就会接管。

Sandboxing 与 approvals 是协同工作的两套不同控制机制。Sandbox 定义了技术边界；approval policy 则决定了 agent 在跨越这些边界之前何时必须暂停并进行询问。

## Sandbox 的作用

Sandbox 不仅适用于内置的文件操作，同样适用于派生（spawned）出的命令。如果 agent 运行类似 `git`、package manager 或 test runner 等工具，这些命令都会继承相同的 sandbox 边界。

Codex 在各操作系统上均采用 platform-native 的强制实施机制。虽然 macOS、Linux、WSL2 和原生 Windows 之间的实现有所不同，但在不同 surface 上的核心理念是一致的：为 agent 提供一个有边界的工作空间，使日常任务能够在明确的限制内自主运行。

## 为什么这很重要

Sandbox 能够减少 approval fatigue（审批疲劳）。agent 无需每执行一个低风险命令都请求你的确认，而是在你预先批准的边界内读取文件、进行编辑并运行常规的项目命令。

它还为 agentic 工作流提供了更清晰的 trust model。你不仅仅是在信任 agent 的意图，更是在信任 agent 始终在受强力约束的限制范围内运行。这使得在放手让 agent 独立工作的同时，依然清楚它会在何时停下来寻求帮助。

## 入门指南

默认的 permissions 模式会自动启用 sandboxing。

### 前提条件

在 **macOS** 上，sandboxing 基于系统内置的 Seatbelt framework 开箱即用。

在 **Windows** 上，在 PowerShell 中运行时 Codex 使用原生的 [Windows sandbox](https://learn.chatgpt.com/docs/windows/windows-sandbox#windows-sandbox)；在 WSL2 中运行时则使用 Linux sandbox 实现。

在 **Linux 和 WSL2** 上，请先使用 package manager 安装 `bubblewrap`：

<Tabs
  id="codex-sandboxing-prerequisites"
  param="sandbox-os"
  tabs={[
    { id: "ubuntu-debian", label: "Ubuntu/Debian" },
    { id: "fedora", label: "Fedora" },
  ]}
>
  


```bash
sudo apt install bubblewrap
```

  


  


```bash
sudo dnf install bubblewrap
```

  

</Tabs>

Codex 会使用在 `PATH` 中找到的第一个 `bwrap` 可执行文件。如果没有可用的 `bwrap` 可执行文件，Codex 会 fallback 到内置的 helper，但该 helper 要求系统支持创建 unprivileged user namespace。安装发行版提供的 `bwrap` 软件包可以保证环境的稳定性。

当缺少 `bwrap` 或 helper 无法创建所需的 user namespace 时，Codex 会在启动时发出警告。在对该 AppArmor 设置有限制的发行版上，建议加载 `bwrap` AppArmor profile，这样可以在不全局禁用该限制的情况下让 `bwrap` 保持正常工作。

**Ubuntu AppArmor 注意事项：** 在 Ubuntu 25.04 上，直接从 Ubuntu 软件源安装 `bubblewrap` 即可正常工作，无需额外的 AppArmor 配置。`bwrap-userns-restrict` profile 已随 `apparmor` 软件包内置在 `/etc/apparmor.d/bwrap-userns-restrict`。

在 Ubuntu 24.04 上，安装 `bubblewrap` 后 Codex 可能仍会警告无法创建所需的 user namespace。复制并加载额外的 profile：

```bash
sudo apt update
sudo apt install apparmor-profiles apparmor-utils
sudo install -m 0644 \
  /usr/share/apparmor/extra-profiles/bwrap-userns-restrict \
  /etc/apparmor.d/bwrap-userns-restrict
sudo apparmor_parser -r /etc/apparmor.d/bwrap-userns-restrict
```

`apparmor_parser -r` 可以在无需重启的情况下将 profile 加载至内核。你也可以重新加载所有 AppArmor profile：

```bash
sudo systemctl reload apparmor.service
```

如果该 profile 不可用或未能解决问题，你可以通过以下命令禁用 AppArmor 的 unprivileged user namespace 限制：

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```



## Permissions 工作机制



使用当前 surface 的 permissions 控制项可以更改 Codex 处理本地操作的方式。

Approvals 决定了 Codex 在执行操作前何时暂停，而 sandbox 则决定了命令可以访问哪些文件和网络资源。当 approval 提供不同作用域选项（例如单次批准或整个 session 批准）时，请选择能让任务继续进行的最小范围（narrowest scope）。建议默认保持在 project 边界内；使用独立的 project 或 worktree，而不是将访问权限扩大到无关的 repository。







在 ChatGPT desktop app 中，可以使用 composer 下方的 permissions 控制项。根据你的配置，该菜单可包含 **Ask for approval**、针对符合条件的审批请求的 **Approve for me**、**Full access**，以及已命名的或自定义 permissions profile。

<PermissionModeSelectorDemo client:load />







<a id="configure-defaults"></a>



## 配置默认值 (Configure defaults)

若要每次启动都保持相同的行为，可以在 `config.toml` 中设置默认值。[Config basics](https://learn.chatgpt.com/docs/config-file/config-basic) 介绍了其工作原理，[Configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference) 记录了 `sandbox_mode`、`approval_policy`、`approvals_reviewer` 以及 `sandbox_workspace_write.writable_roots` 的具体配置项。可以使用这些设置来决定 agent 默认拥有的自主权程度、允许写入的目录、何时暂停等待 approval，以及由谁来审查符合条件的 approval 请求。

概括而言，常见的 sandbox mode 包括：

- `read-only`：agent 可以查看文件，但未经 approval 无法编辑文件或运行命令。
- `workspace-write`：agent 可以读取文件、在 workspace 内部进行编辑，并在该边界内运行常规的本地命令。这是本地工作中默认的低阻力（low-friction）模式。
- `danger-full-access`：agent 运行时不受 sandbox 限制。这将移除文件系统和网络边界，仅在希望 agent 拥有完全访问权限时使用。

常见的 approval policy 包括：

- `on-request`：agent 默认在 sandbox 内工作，并在需要超出该边界时主动询问。
- `never`：agent 不会停下来等待 approval 提示。

Codex 和 ChatGPT Work 不再支持将 `untrusted` 作为可选的 approval policy。如果现有配置使用了该值，请参阅 [从已废弃的 `untrusted` approval policy 迁移](https://learn.chatgpt.com/docs/agent-approvals-security#migrate-from-the-retired-untrusted-approval-policy)。

当 approval 处于交互式状态时，你还可以通过 `approvals_reviewer` 选择由谁来审查它们：

- `user`：approval 提示会呈现给用户。这是默认设置。
- `auto_review`：符合条件的 approval 提示会被发送给 reviewer agent（参见 [automatic review](https://learn.chatgpt.com/docs/sandboxing/auto-review)）。

Full access 意味着将 `sandbox_mode = "danger-full-access"` 与 `approval_policy = "never"` 配合使用。相比之下，低风险的本地自动化预设是将 `sandbox_mode = "workspace-write"` 与 `approval_policy = "on-request"` 配合使用，或使用对应的 CLI flag `--sandbox workspace-write --ask-for-approval on-request`。随后，你可以保持 `approvals_reviewer = "user"` 进行人工 approval，或者设置为 `approvals_reviewer = "auto_review"` 进行自动化 approval 审查。

如果需要 agent 跨多个目录工作，writable roots 允许你在不完全移除 sandbox 的情况下扩展其可修改的路径。如果需要更宽或更窄的 trust boundary，请调整默认的 sandbox mode 和 approval policy，而不是依赖一次性的例外规则。

当工作流需要特定的例外规则时，可以使用 [rules](https://learn.chatgpt.com/docs/agent-configuration/rules)。Rules 允许你在 sandbox 之外 allow（放行）、prompt（提示）或 forbid（禁止）指定的命令前缀（command prefixes），这通常比盲目扩大访问权限更为合适。有关 IDE 特有的设置入口，请参阅 [Codex IDE extension settings](https://learn.chatgpt.com/docs/developer-settings?surface=ide)。

Automatic review（如果可用）不会改变 sandbox 边界。它只是该边界上处理 approval 请求（例如 sandbox escalation、受阻的网络访问或仍需批准的带副作用 tool call）时的一种可选 `approvals_reviewer`。sandbox 内已允许的操作仍会直接运行而无需额外审查。有关 reviewer lifecycle、trigger types、denial semantics 以及配置详情，请参阅 [automatic review](https://learn.chatgpt.com/docs/sandboxing/auto-review)。

各平台的具体细节请查阅对应平台的文档。有关原生 Windows 的安装配置、运行行为和故障排除，请参阅 [Windows](https://learn.chatgpt.com/docs/windows/windows-sandbox)。有关管理员要求以及组织级别对 sandboxing 和 approvals 的约束，请参阅 [Agent approvals & security](https://learn.chatgpt.com/docs/agent-approvals-security)。