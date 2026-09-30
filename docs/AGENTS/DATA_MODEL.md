# 数据归属与恢复

数据归谁，取决于谁负责写入、校验和恢复，不取决于它显示在哪个页面。运行数据在当前用户的 `~/.harness`；Web 与 Desktop 启动前取得同一把 `.lock` 跨进程锁。`persist` 只提供限定路径、同步追加和原子替换，格式及坏数据处理归各领域。

## 存储位置

```text
~/.harness/
├─ config.yaml                       LLM 供应商、API Key、Jev 配置
├─ models.json                       用户模型覆盖与隐藏项；内置 catalog 随程序发布
├─ model-auth/{codex,xai}.json       内置账号凭据
├─ mcp.json / mcp/oauth/             全局 MCP 配置与独立 OAuth 凭据
├─ approvals/{settings,mcp-trust}.json 审核设置、项目 MCP 信任
├─ hooks/                            全局 Hook 设置与项目配置摘要信任
├─ agents/ / skills/ / skills.json   Agent、个人 Skill 与开关
├─ reading/<session-id>.json         单调前进的本机已读位置
├─ subagents/tasks/<task-id>.json    父子关系与委派，不重复保存子 Run 状态
├─ deletions/<intent-id>.json        永久删除意图；重启先重放清理
├─ desktop/window.json               Desktop 窗口布局；坏文件回退默认布局
└─ sessions/<session-id>/
   ├─ meta.json                      标题、titleEdited、可选 archivedAt
   ├─ settings.json                  Agent、模型、档位、工作区、权限模式
   ├─ messages.jsonl                 追加式对话账本
   ├─ runs.json                      Run 身份、状态、锚点、用量与 Diff 摘要
   └─ diffs/<run-id>.json.gz          Run 的文件前后内容及 revision
```

项目文件不属于这棵用户数据目录。项目 MCP／Hook 配置在项目内，信任记录在 `~/.harness`；永久删除项目只删 Harness 会话与相关信任，不删真实项目目录。模型目录只接受当前格式，不迁移旧数据；内置快照不复制进用户文件。

## 事实主人

| 事实 | 主人 | 边界 |
| --- | --- | --- |
| 对话 Entry、分支、标题和归档 | `session` | `titleEdited` 防止首条消息覆盖手动标题；归档不停止活 Run |
| 本轮设置 | `session/settings` | 分叉复制；子任务继承父 Run 快照；单次批准不写入设置 |
| 活 Run、草稿、终态、Diff | `runner` | 活状态在内存；终态和 Diff 独立于对话账本 |
| 子任务 | `subagents` | Task 只存关系与委派；结果从子 Session 和 Run 派生 |
| 待审批和批准 | `approvals` | 待办只在内存；断线不取消 Run，重启不恢复等待 |
| 长期 Agent 进程 | `machine/local` | 跨 Turn 存活、进程退出清理；不写账本 |
| 用户终端 | Client 连接与 machine | 断线终止；不与 Agent 进程表或 Session 生命周期混用 |
| 页面投影、草稿、折叠和主题 | Client | 草稿按会话保存在内存；刷新不承诺恢复 |

`messages.jsonl` 每行一个带 `id`、`parent`、`seq`、`body` 的 Entry；`parent` 是分支关系，`seq` 只在落账时分配。`body` 可含 text、image、reasoning、tool-call、tool-result、summary；工具调用与结果按 ToolCall.ID 配对，结果仍有自己的 Entry.ID。协作消息记录可信来源，只进入直属父账本，发给模型时作为普通输入，不提升为系统指令。上下文引用是用户 text 末尾的版本化文本，不另建引用表。

Runner 的流式草稿先作为事件发送，完整 Entry 落账后才发布耐久变化；`runs.json` 与账本分开。未完成 Run 在重启后标为 interrupted，不自动续跑。Stop 不落账，未落账的 Steer 被拒绝；协作回报以父账本 MessageID 确认，否则由来源重试。Snapshot 与订阅以 epoch／序号衔接，Client 按 Entry.ID 去重，序号缺口重新订阅。

助手 reasoning 或 tool-call 可保存模型续接数据；只在同供应商、模型、协议下重放，不能把它当 OAuth 凭据。内置账号与 MCP OAuth 凭据不进入 RPC 或日志；Windows 用当前用户 DPAPI、Unix 用私有文件权限保护。令牌刷新先保存再使用。项目 MCP 信任按真实路径与有效配置摘要，Hook 信任只覆盖配置版本，不覆盖脚本内容。

会话已读位置由 `reading` 根据实际完成的 Run 单调推进；旧确认不能抹掉新结果。永久删除先写意图再清理会话、子任务与相关读标，失败留待重启重放。单个损坏的会话元数据或设置目前会阻断整个会话列表，修复方向见 [STATUS](STATUS.md)。

新增字段前先问：谁的事实、是否耐久、谁需要读取、故障后如何恢复。
