# 数据模型与归属

面向后续维护 Harness 的人和 AI。先确认一件事：数据放在哪里，取决于它是谁的事实；不是取决于它显示在哪个页面。

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
│
└─ sessions/<session-id>/
   ├─ messages.jsonl
   │  对话账本
   ├─ meta.json
   │  会话元数据
   └─ settings.json
      此会话的运行设置
```

旧项目内 `.harness-data/` 和用户目录根下旧平铺会话文件都不再读取，可由用户自行删除。

## 谁拥有什么数据

```text
Session
└─ 对话发生过什么
   用户、助手、工具调用、工具结果；可分叉

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

Subagents
└─ 父子 Session 关系、稳定任务 ID、每轮 RunID 与结果 EntryID、错误和通知
   子会话仍使用普通账本及 SessionSettings，但不进入普通聊天列表或空会话复用
   重启只恢复记录，将未完成轮次标记中断，不自动启动

浏览器状态
└─ 当前设备上的临时界面状态
   面板开关与宽度、折叠展开状态、主题偏好
```

## 不可跨越的边界

```text
Session
  只记对话事实
  不写 Todo、审批、Dock 状态、面板状态、运行时间

插件状态
  插件自己拥有、自己保存、自己恢复
  不借 Session 当通用存储

浏览器状态
  不写 Session
  刷新后可丢失的状态不必持久化

运行事件
  Runner 通过 SSE 通知屏幕
  不是账本，也不是插件存储

Skill 正文
  保留在各自 Skill 目录的 SKILL.md 和相对资源中
  Prepare 只把摘要与 SKILL.md 绝对路径写入本轮提示词；模型按需使用已启用的普通 Tool 读取正文
```

## 对话账本

`messages.jsonl` 是追加式账本，每行一条 `Entry`：

```text
id      这一条是谁
parent  接在前一条哪里；支持分叉
seq     全局写入顺序
body    本条事实：role、runID、blocks
```

```text
messages.jsonl
├─ #1 用户文本
├─ #2 助手推理 + 回答
├─ #3 用户文本
├─ #4 助手工具调用
├─ #5 工具结果
└─ #6 助手最终回答
```

`blocks` 只记录实际发生的对话内容：`text`、`reasoning`、`tool-call`、`tool-result`、`summary`。页面长什么样、哪些内容展开，不是账本事实。`summary` 是压缩落账的助手块；`History()` 把它收成普通文本再发给模型。

## 修改前四问

新增一种数据前，先回答：

```text
1. 它是谁的事实？Session、会话设置、Agent、插件，还是浏览器？
2. 重启后必须恢复吗？
3. 是否需要被其他插件读取？
4. 它是耐久事实，还是本轮运行的临时通知？
```

答案明确后再选 Store、事件或浏览器状态；不要为了页面方便，把数据写进错误的主人。
