# 数据模型与归属

面向后续维护 Harness 的人和 AI。先确认一件事：数据放在哪里，取决于它是谁的事实；不是取决于它显示在哪个页面。

`products/harness` 只组合现有 Session / SessionSettings / Runner / Subagents 的业务，不迁移用户数据。`clients/contracts/` 是手工维护的 TS 接口类型；appserver 的 Schema 只用于运行时校验，不提供接口目录。这些契约都不是运行状态或持久化格式；对外 Session 投影中的时间编码为 RFC 3339 字符串。

## 物理位置

运行数据根目录固定为当前用户的 `~/.harness`，不写入项目目录。

```text
~/.harness/
├─ config.yaml
│  全局 LLM 配置
│
├─ default.agent.json
│  新会话默认使用、可编辑但不可删除的 Agent 配置
├─ <agent-id>.agent.json
│  其他用户创建的 Agent 配置
│
├─ subagents/tasks/<task-id>.json
│  委派关系、逐轮状态与结果位置、待投递通知；版本化 JSON 原子替换
│  通知的 delivered 只在 Runner 确认父账本已有该消息 ID 后保存；失败保留待重试
│
└─ sessions/<session-id>/
   ├─ messages.jsonl
   │  对话账本
   ├─ meta.json
   │  会话元数据
   ├─ settings.json
   │  此会话的运行设置
   └─ runs.json
      各轮运行身份、状态、锚点与错误；不重复保存消息正文
```

旧项目内 `.harness-data/` 和用户目录根下旧平铺会话文件都不再读取，可由用户自行删除。

## 谁拥有什么数据

```text
Session
└─ 对话发生过什么
   用户、助手、工具调用、工具结果；可分叉
   collaboration 是带来源的协作消息，不是用户输入或系统指令

SessionSettings
└─ 这场会话怎样运行
   Agent、模型、思考档位、工作区

Agent 设置
└─ 一个 Agent 怎样工作
   Kind、System Prompt、允许的普通 Tool；不进对话账本

Skill 发现
└─ 文件系统上的 Skill 定义，不复制进 Agent 或 Session
   系统、个人与当前项目 Skill 对该作用域所有 Agent 自动可用

插件状态
└─ 插件自己的业务事实
   Todo、审批、游戏状态、插件设置等

Runner 运行结果
└─ 每轮 RunID、状态、账本锚点和错误；与对话正文分开
   生成中草稿只在 liveRun 内存，不写硬盘
   重启把未收尾的 running 标为 interrupted，不自动续跑

Subagents
└─ 父子 Session 关系、稳定任务 ID、每轮 RunID 与结果 EntryID、错误和通知
   子会话仍使用普通账本及 SessionSettings，但不进入普通聊天列表或空会话复用
   重启只恢复记录，将未完成轮次标记中断，不自动启动
   List 返回 TaskView 查询投影：按结果 EntryID 从子账本读取最终正文，不把正文重复存入任务 JSON
   停止代次、被停止的父 RunID 与孩子停止标记仅在服务内存，拦截停止前的在途派生操作
   不另建协作级 Context；实际执行取消沿用 Runner，关闭仍由服务自身生命周期负责

Client 状态
└─ 当前设备上的临时界面状态与后台投影
   面板开关与宽度、折叠展开状态、主题偏好

app-server 瞬时状态
└─ 连接、JSON-RPC 请求响应配对、订阅和待回答请求
   不进入 Session，也不是业务防重记录
   初始化状态、待发队列和订阅读快照期间的缓冲也只存在内存
   每个 Connection 只拥有一个 Client 的基础设施连接状态，断线后整体失效
   不保存 Session、Run、设置或产品状态，也不参与任何业务判断
```

## 不可跨越的边界

```text
Session
  只记对话事实
  不写 Todo、审批、Dock 状态、面板状态、运行时间

插件状态
  插件自己拥有、自己保存、自己恢复
  不借 Session 当通用存储

Client 状态
  不写 Session
  刷新后可丢失的状态不必持久化
  当前会话 ID 只记在本标签页 sessionStorage；输入草稿／图片预览按会话留内存
  Snapshot 本身是投影底稿，实时更新同一份 entries / runs，不另存前端账本

运行事件
  Runner 产生稳定事件，app-server 按订阅投影给 Client
  不是账本，也不是插件存储
  生成中的正文／思考草稿只在 liveRun 内存；完整消息先落账，再移除同 Entry.ID 草稿
  运行结果（身份、状态、锚点、错误）由 Runner 写入 runs.json，不伪造结束消息

连接与请求
  JSON-RPC 请求 ID 只匹配一次响应；连接、订阅和待发送队列都在内存
  send.expectedRunID 只是本次插话的身份前提，不新增消息身份或持久化字段
  Connection 只是 IM 网关式的临时连接对象，不能成为业务状态或业务规则的主人
  后台重启后全部失效，Client 必须重新初始化，通过订阅接口一起取得 Snapshot 与后续事件
  Snapshot 与通知的重叠：耐久消息按 Entry.ID 去重；实时增量按本进程会话更新序号过滤快照边界之后的事件
  更新序号不能跨会话或跨后台重启混用；重启后未收尾的运行标记中断，不自动续跑
  状态与更新序号一起提交；网络订阅按序号整理乱序，快照已含的更新不再发送
  runs.json 的读改写与快照互斥；快照不能用旧记录覆盖新结果

Skill 正文
  保留在各自 Skill 目录的 SKILL.md 和相对资源中
  Prepare 只把摘要与 SKILL.md 绝对路径写入本轮提示词；模型按需使用已启用的普通 Tool 读取正文
```

## 对话账本

`messages.jsonl` 是追加式账本，每行一条 `Entry`：

```text
id      这一条是谁；生成开始时分配，增量、草稿和落账共用，不靠 stepSeq 对应
parent  接在前一条哪里；支持分叉
seq     全局写入顺序，只在实际落账时分配
body    本条事实：role、runID、blocks；协作消息另带 messageID、sourceSessionID、sourceRunID
        未完成助手消息带 incomplete；生成开始时的账本锚点为 afterSeq
```

Run 的起点与单条输出的位置分开：已开始的输出不会因 Steer 移位；检查点消费新输入后，新输出的 `afterSeq` 才前移。

```text
messages.jsonl
├─ #1 用户文本
├─ #2 助手推理 + 回答
├─ #3 用户文本
├─ #4 助手工具调用
├─ #5 工具结果
└─ #6 助手最终回答
```

`blocks` 只记录实际发生的对话内容：`text`、`reasoning`、`tool-call`、`tool-result`、`summary`。页面长什么样、哪些内容展开，不是账本事实。`summary` 是压缩落账的助手块；`History()` 把它收成普通文本再发给模型。未完成消息保留半截正文与思考，并附「未完成」说明；不把思考改成普通正文，不携带悬空工具调用。工具结果按 `ToolCall.ID` 回填，工具结果消息有自己的 Entry.ID。

协作消息在账本使用 `role=collaboration`，`runID` 是接收它的父 Run，`sourceSessionID/sourceRunID` 是孩子的来源。启动前失败没有真实子 Run，来源 RunID 留空，不捏造身份。发给模型时转换成带来源说明的普通输入，不提升为系统指令。通知重试按父账本中实际存在的 `messageID` 去重，不靠内存中的“已发送”判断。

## 修改前四问

新增一种数据前，先回答：

```text
1. 它是谁的事实？Session、会话设置、Agent、产品/插件、app-server，还是 Client？
2. 重启后必须恢复吗？
3. 是否需要被其他插件读取？
4. 它是耐久事实，还是本轮运行的临时通知？
```

答案明确后再选 Store、事件或 Client 状态；不要为了页面方便，把数据写进错误的主人。
