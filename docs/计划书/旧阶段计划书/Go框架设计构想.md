# edith-harness：Go Agent 框架设计构想

> 目标：保留 dsh 的"任意可扩展"能力，去掉它的包袱。
> 不做：动态插件、AI 自我修改、热重载、Web 分体架构。
> 要做：一个开发者 **30 分钟能看懂、一天写出第一个插件**的 Go 框架。
> 本版已合并外部审核（GPT-5.6）修正：事件失败语义、process 座、持久化契约、作用域边界表述。

## 命名规范（可读性第一）

1. 包名 = 它管的那个东西：`session`、`tools`、`files`、`process`、`loop`；
2. 方法名 = 日常动词且自解释：`Subscribe`、`Broadcast`、`RegisterTool`、`ExecuteCall`、`SubmitFollowup`；
3. 不发明概念名：Kernel/Ledger/Fiber/Realm 一类词出现即毙；
4. 避开标准库撞名：`context`、`runtime`、`log`、`sys` 不作包名。

| 概念 | 定名 |
|---|---|
| 内核 | `core`（App + 事件 + 作用域）|
| 运行时容器 | `App`（组合器：插件从它取服务、挂监听）|
| 插件 | 结构体 + `Name()` + `Start(app)`；清理用 `app.OnCleanup(fn)` |
| 会话日志 | `session`（必装插件，紧跟唯一 Journal 插件；内核 Kind 走窄写入口，公开 Append 只给插件自定义 Kind；`s.ModelHistory()`）|
| 压缩 | 摘要事件带 `Replaces: [3,6]` 字段 |
| 中间件链 | `RunChain`（原 waterfall）|
| 主循环 | `loop` 包（`loop.Driver`）|
| 执行能力 | `files.Store` + `process.Spawner` 底座；`shell.Runner` 建在 process 之上 |

方法命名纪律：**自解释动词，看到名字就知道职责**，不为短而牺牲可读性。

代码组织约定（每包通用）：

- `module.go` 包入口与生命周期；`<capability>.go` 一个能力一个文件、方法跟着能力对象走；`types.go` 只放数据契约（会 JSON 传输/落盘的类型）；`state.go` **按需出现**——仅当多个能力对象需要共享内部状态（如同一把锁）时才建，不为凑数空挂；
- 结构体三角色：数据载体（types.go）/ 能力对象（自有状态+方法）/ 组合器（只组装和调度）；
- 每包一个公开构造入口（New/Build/Load/Create 四选一），构造只做依赖校验+组装；
- 闭包只用于无状态一次性包装（如中间件适配），禁止用闭包隐藏长期状态；
- 能力对象不回指组合器，共享状态抽成内部 state 叶子对象。

---

## 一、核心思想

**把内核做小，小到做完就不需要再改。** 内核行数是目标不是硬约束——不变量（模型可见 ⊆ 已记账、权限只收不放）靠测试保证，不靠信任插件自觉：自定义 loop 也必须经过 session/tools 契约，不能绕过账本直接调模型。

之后所有新能力都通过"加插件"实现。内核稳定，外围扩展。

## 二、内核只有 5 个模块

### 1. 事件总线（core 内）

四种分发方式，**每种有自己的失败语义**（不许一刀切）：

| 方式 | 语义 | 失败语义 |
|---|---|---|
| `Broadcast` 广播 | 通知，不等待 | 观察者 panic 隔离为日志，不影响主流程 |
| `RunChain` 链式 | 中间件链，可改可拦 | 错误向上传递；安全场景失败即拒绝 |
| `RunConcurrently` 并行 | 全部并发，等全部完成 | 聚合失败并返回错误（如多方并行扇出）|
| `RunSequentially` 串行 | 按注册顺序执行 | 错误中断并返回 |

工程约束：Broadcast 为**同步分发**（按注册顺序直接调用，快监听器），不逐事件起 goroutine，避免流式输出触发 goroutine 风暴；需要异步的监听器自己声明。

### 2. 会话日志（session）

`session` 通过 `session.Plugin` 接入 App：持久化插件先登记可替换的 `"journal"`，session 再领取它、创建并登记 `"sessions"` Store。插件化不等于可选；不能省略 session，只能替换 Journal 插件。

**只追加、不修改**的事件序列，是整个系统唯一的事实源：

- 模型看到的对话历史由日志**投影**得出，不单独存储；
- UI 显示的是日志的另一种投影；崩溃恢复 = 重放日志；
- 上下文压缩 = 追加摘要事件，带 `Replaces: [5, 30]`（旧日志不删，投影时跳过）；
- 事件类型开放：插件注册新事件类型（编解码器），不改 session 包。

**持久化契约（第一行日志落盘前锁定）**：

- 会话格式版本号（header 一份）；
- 每类事件可带自己的版本；
- 每类事件声明 required / ignorable：未知 required 事件 → 拒绝重建（宁可过严），未知 ignorable 事件 → 保留跳过；
- 迁移入口：旧版本事件 → 新版本的迁移函数表；
- 投影检查点（水位）：长会话不全量重放，从检查点续算；chunk 事件可合并存储。

**已知边界**：重放确定性保证**状态重建**一致，不承诺**重跑执行**一致（模型采样、网络、文件、时间都会变）。fork 回到过去 ≠ 重演得到相同未来。

### 3. 作用域（Scope）

规则只有一条：**所有注册项要么是全局的，要么属于某一个 agent。**

- 同名时作用域内注册**遮蔽**全局——给单个 agent 定制工具集和人格的方式；
- 父子关系只是数据，不影响可见性；
- **边界声明：作用域对模型是硬边界**（注册表里没有 → 调用即 UNKNOWN_TOOL）；**对同进程代码不是安全边界**——恶意插件仍能读任意内存。真正的隔离靠进程/沙箱（多租户不可信用户必须独立进程）。

实现纪律：注册表内部不把"固定两层 map"暴露为公共契约，为将来引入 preset 父层留空间（当前先实现两层）。

### 4. 主循环（loop）

turn（一轮）> step（一次模型请求 + 工具执行），由 `loop.Driver` 驱动。输入三种：`SubmitFollowup`（开新轮）、`SubmitSteering`（下一步骤消费）、`InjectContext`（只带上下文不唤醒）。主循环本身是插件，可整体替换。

### 5. 组装（cmd/edith-harness + config）

v1 用一份极简 YAML 选择已经编进程序的大零件：档案存储、账本、模型适配器、执行环境、工具插件和 Runner。`session`、`llm`、`tools`、`agents` 四个固定插座自动安装；同一内核，不同组装 = 不同产品。

## 三、开发体验：硬指标

第一个插件 20 行以内：

```go
// 拒绝在周末执行删除类工具
type WeekendGuard struct {
    tools *tools.Registry // 显式依赖：只拿需要的能力对象
}

func New(tools *tools.Registry) *WeekendGuard { return &WeekendGuard{tools: tools} }

func (WeekendGuard) Name() string { return "weekend-guard" }

func (g *WeekendGuard) Start(app *core.App) error {
    g.tools.OnBeforeExecute(g.denyWeekendDeletes)
    return nil
}

func (g *WeekendGuard) denyWeekendDeletes(call tools.Call, next func() tools.Decision) tools.Decision {
    if isWeekend() && call.Name == "file_delete" {
        return tools.Deny("周末不删文件")
    }
    return next()
}
```

验收标准：30 分钟讲清 5 个模块和 4 种分发；半天写出第一个工具和监听插件；一周定义一个新 Service 并让模型用上。

## 四、插件的四种扩展层级（全部支持）

| 层级 | 做法 | 代码量 |
|---|---|---|
| 替换已编进程序的大零件 | 改 YAML 名字 | 零代码 |
| 增加工具 / 添加拦截规则 | 写插件：注册监听器或工具 | 一个函数 |
| 新增能力（视频、游戏 AI…） | 定义新 Service，与 tools/llm 同级 | 一个接口 + 实现 |
| 新增事件类型 / 消息内容格式 | 注册编解码器 | 少量代码 |

四个层级可自由组合。唯一约束：**只依赖接口定义，不依赖具体实现**。

### 扩展考验（压力测试记录，均已验证内核零改动）

| 考验 | 方案 | 踩中的机关 |
|---|---|---|
| 辩论会主循环（双模型互评）| 换 loop 插件 | loop 可替换 |
| AI 狼人杀（信息隐藏）| 新 Service + 每玩家作用域 | 作用域控制**模型可见性**（同进程代码需进程隔离）|
| 时间旅行调试（fork 重跑）| fork 账本前缀 + InjectContext | 事件溯源 + 回放确定性（重放≠重跑）|
| 小说工作室（多 agent 协作）| 五个 agent 各自作用域 + steering | 多作用域 + 三种投递 |
| 邮件 / IRC / 门铃驱动 | 传输桥接插件翻译成 SubmitFollowup | 投递入口统一 |
| 多租户服务器 | 薄宿主管 N 个 App；App 懒加载 + 闲置回收 + 资源配额；**不可信租户独立进程** | App 可多实例 + 执行环境插座 |
| 本地客户端 + 服务端双产品 | 同一内核，两个宿主、两套组装 | 组装机制 |
| ACP 编辑器协议 | 翻译插件 + `--acp` 模式；编辑器文件操作 = files.Store 新实现；stdio 进程 = process.Spawner | files/process 插座 + 审批应答器留缝 |

UI 桥接协议（命令面 + 事件流）做**版本化**，桌面桥与服务器桥共用同一份契约，防止半年后演化成两套 API。

多租户前置纪律（v1 就要守）：无包级全局变量；插件 Start 只碰自己的 App；碰 OS 一律走接口；用量进账本。

## 五、关键技术决策

| 问题 | 决策 | 理由 |
|---|---|---|
| Go 没有声明合并，如何开放事件类型？ | 事件实现 `Kind() string` + 编解码器注册表 | 词汇开放、代码封闭 |
| 插件启动顺序？ | YAML 只选择大零件，Go 代码按固定依赖顺序安装；中途失败逆序回滚已启动插件 | 不让配置自行排序，避免把依赖错误留给用户 |
| 任务取消？ | `context.Context` 全链路 | 语言原生 |
| 执行环境分几个座？ | **`files.Store` + `process.Spawner` 两个底座，`shell.Runner` 建在 process 上**；两者必须来自同一执行环境实现 | LSP/PTY/ACP/子 agent 都需要长生命周期进程、stdio、信号、进程树；一次性 shell 命令接不住（dsh 同样分 fs/subprocess/shell 三 seam）|
| 本地 vs 沙箱？ | 本地实现一个插件同时提供 files + process；沙箱实现替换两者 | 换执行环境，工具零改动 |

复杂度控制三纪律：契约要少且稳；事件词汇要节制；只有一个实现时不抽接口。

## 六、目录结构

```
harness/
  core/      App + 事件总线 + 作用域（零依赖）
  chat/      对话词汇表：Message / 内容块 / 用量（session 与 llm 共用）
  session/   会话事件日志 + 压缩 + 投影 + 持久化契约
             必装插件：`"journal"` → Store → `"sessions"`
  persistence/  持久化插件（一种介质一个目录，二选一）
    jsonl/       JSONL Journal 插件
    sqlite/       将来 SQLite Journal 插件
  loop/      主循环 Driver + 输入提交 + 取消
  tools/     工具注册表 + 执行链（五道关卡）
  llm/       模型适配接口 + 流式协议
  workspace/ 工作空间能力（不是插件）
    files/     文件能力接口（不碰本地磁盘）
    process/   长期进程能力接口（不启本地进程）
    shell/     站在 process 之上的通用一次性命令
  localenv/  本地 files + process 实现，并成对组装 files + process + shell
  std/       第一批插件：bash / 文件 / 网络 / 技能 / 子 agent / 待办
  cmd/edith-harness/   main：组装 + 配置
```

依赖方向单向：`core ← workspace/files`、`core ← workspace/process ← workspace/shell`；`localenv` 只负责把三者成对组装。`persistence/jsonl` / 将来的 `persistence/sqlite` 都依赖 `session.Journal`，但不反过来被 session import。将来的沙箱另做实现，不改工具。

## 七、明确不做

- ❌ 动态加载插件（编译期确定，单二进制）
- ❌ AI 自我修改
- ❌ 热重载
- ❌ Web 前后端分体、通用 RPC 网关（UI 走版本化桥接协议 + 渲染意图；将来可加 Wails 桌面壳）
- ❌ 多语言文档、60 包分组管理

## 八、实现顺序（每步可独立验收）

1. **core**：事件总线（四种失败语义）+ 作用域 + Chain
2. **chat + session**：中立词汇包 + 追加契约 + **持久化版本契约** + 压缩 + 投影检查点（附回放确定性测试）
3. **llm + tools**：适配器 + 五道关卡
4. **loop**：最小主循环 + 三种输入提交 + 取消
5. **files + process + shell**：接口 + 本地执行实现 + 行为测试
6. **cmd/edith-harness**：组装配置 + `--dump-config` + 真实闭环冒烟（真实模型 + 副作用工具 + 重启恢复）
7. 之后全部是插件：std 工具集（bash/文件/网络/技能/待办）、沙箱、压缩、审批、子 agent、ACP……

v1 冻结前用两个模型适配器、两个执行后端、一个子 agent 插件做验证，再承诺"内核不改"。

> 核心原则：**前 6 步完成后，内核即告完工。**
> 之后的所有演进都通过插件完成——内核不再为新能力改一行代码。

## 九、写码途中盯的 & 已识别风险

- **每个 App 必须有完整幂等的 `Close()`**：租户卸载后 goroutine、连接、进程、订阅归零；
- **渲染意图带版本 + 通用降级**：旧 UI 遇到新卡片至少显示原始文本；
- **工具副作用未知窗口**：`tool/start` 之后崩溃 = 副作用窗口 = **待裁决**（禁止自动重跑，裁决须落成 tool/result 才继续）；无 start = 未开跑可 skipped；外呼类工具建议记录幂等键（以《失败边界与恢复状态机》为准）；
- **插件退役兼容**：移除插件前，其历史事件的编解码器保留在 compat 包或提供迁移器，否则旧会话无法恢复；
- **慢消费者背压**：每条事件流明确策略（阻塞/丢弃/合并/断开），UI 断线不能撑爆内存；
- **长会话性能**：chunk 合并、投影检查点、分页，否则日志正确但越来越慢。
