# M2 计划书 —— session 包（账本）

> 状态：已定稿（经第一性原理评审：砍掉 Fork、合并存活名单、明确记账判据）。
> 本文是施工依据，改本文 = 改设计。

## 职责（一句话）

**记住发生过的一切，并让所有人从同一份记录里各取所需。**

M2 不懂 agent 知识——不知道"模型"是什么、不知道谁在记账。它是一本规矩很硬的账本。

## 不做什么（边界）

- ❌ 不管谁记账——**记账权按 Kind 单主分配**：tools 独占 tool/call、tool/start、tool/result；loop 独占投递/领出/消息/chunk/快照/轮步——全部走窄写入口（终审 P1，五轮复核补 start）
- ❌ 不管压缩策略——何时压、摘要写什么，是将来的压缩插件的事；M2 只认识 `Replaces` 标记
- ❌ 不管持久化后端——JSONL/SQLite 是将来的插件；M2 只保证"无损进出 + 版本规矩"
- ❌ 不管人看的排版——那是 UI 的投影；本包的翻译是机器对机器
- ❌ **不做 Fork**（本次评审砍掉）——`Create(seed)` 原语保留，Fork 只是它的组合用法，需要时再加（纯增量）
- ❌ 不做成可拔插件——它是内核六件之一，"模型可见 ⊆ 已记账"的命根由它扛

## 核心判据：什么才记账

> **要"复活"的才记：模型看到的一切（铁律）+ 重启后还要在的东西。**
> 过眼云烟不记：缓存的视频帧、解析树、UI 刷新、中间变量。

---

## 四个功能

### ① 记账（唯一入口，规矩极硬）

```go
type Event struct {
    Kind         string          // "user/message" / "tool/call" ...
    Seq          int             // = 账的长度，必须连续
    Time         int64
    Data         json.RawMessage // 记账时校验 + 深拷贝（防改）
    Replaces     []int           // 摘要事件专用："我取代了这些编号"
    SkipIfUnknown bool           // 写我的插件被卸载后：true=跳过我照常读账
                                 // false=拒读整本账（宁严勿猜）
}

func (s *Session) AppendEvent(kind, data) Event
```

三条铁律：**只增不改、编号连续、记账时验**（非法 JSON 当场报错，不留给硬盘）。
记账流程：**分配 Seq → 校验/编码 → Journal 原子提交 → 内存发布 → Broadcast**，同一 Session 内线性化（单写者），广播只发生在提交成功之后。

**Journal 是原子提交边界，不只是同步钩子（红队终审）**：
- Append 成功 = 该 Seq 已按声明的耐久级别落盘（关键事件 fsync）；
- Append 失败后，介质上**不得留下半条事件**——实现须保存写前偏移、writeAll 完整写入、任何失败截回写前偏移；
- 每条记录携带可校验的 Seq 与完整性信息，重开时验证连续性；**只允许修复可证明未提交的最后一条残尾，中段损坏必须拒绝**；
- 落盘粒度：用户消息、收件箱、tool/start、tool/result 同步写；chunk 允许按步刷新，取消时已收前缀必须已在盘上；崩溃丢**当前步未刷新的 chunk 后缀**可接受（关键事件无此豁免）。

**受保护 Kind 的写入口（红队终审）**：内核 Kind（tool/call、tool/start、tool/result、轮/步、消息、chunk、投递/领出、快照）不走公开的通用 Append——session 向 tools/loop 各发**窄写入口**（类型化方法），插件自定义 Kind 走单独注册的扩展入口；内核 Kind 的编解码器归 session，tools/loop 只调类型化方法。"Kind 单主"由 API 强制，不靠文档约定。
**压缩藏在 `Replaces` 里**：摘要事件声明取代谁，旧账一字不动。

### ② 算历史（同一本账，两种读法）

```go
func (s *Session) ModelHistory() []Message  // 模型视角：跳过被取代的
func (s *Session) Events() []Event          // 全量视角：人 / 审计 / 回放
```

内部一张**存活名单**（哪些编号还没被取代）+ **算过的不重算**缓存（取代发生时缓存作废重算）。

**投影规则（终审 P0）**：每种 Kind 在编解码器里声明"投影为 chat.Message / 不投影"，**默认不投影**；连续 chunk 折叠成一条 assistant 消息；收件箱投递、请求快照、轮/步标记只进 `Events()`，不进模型历史。**被中断的回复只落一条**：interrupted 消息即 chunk 的折叠结果（或 Replaces 掉这些 chunk），不得既折叠又另写一条（五轮复核）。请求快照是"当年发出去的那份"的独立投影，不是 ModelHistory 的一部分。

### ③ 翻译与版本（祖传账本的规矩）

- 每种事件注册一个编解码器：结构体 ↔ JSON，无损往返；
- **不认识的事件类型**：默认拒读整本账（宁严勿猜）；带 `SkipIfUnknown` 标记的 → 保留跳过；
- 账本封面带**格式版本**（整本账一个版本号就够）；迁移函数表留入口（先空着）；将来某类事件需要独立演化时再加事件级版本（纯增量，旧账=版本0）；
- 事件类型对插件开放：插件注册自家 Kind + 编解码器即可记账（todo、goal、视频摘要……）。

### ④ 名册（管家）

```go
func (st *Store) Create(id string, seed ...Event) *Session  // 新账；带 seed = 回放旧账
func (st *Store) Get(id string) *Session
```

---

## 文件与体量

```
session/
  journal.go   Journal 原子提交接口（内存实现给单测，JSONL 到 M6 接）
  store.go     管家：名册 Create/Get + seed 回放；构造 NewStore(journal, broadcaster) 并列注入
  session.go   塔本体：AppendEvent / ModelHistory / Events
  codec.go     翻译官：结构体 ↔ JSON + 未知事件策略 + 版本
  history.go   算历史 + 存活名单 + 增量缓存（名单是算历史的内部状态，不独立成文件）
  types.go     数据格式：Event / Header / 各事件字段
```

五个文件，估计 ~400 行。

**前置新增中立词汇包 `chat/`**（本里程碑建立）：Message、内容块、工具调用块、用量——session 与 llm 的公共词汇，消除"M2 用 M3 类型"的前向引用（外部审核修正）。

## 裁决记录（为什么是这样）

| 问题 | 裁决 | 理由 |
|---|---|---|
| 记什么 | 要复活的才记 | 模型可见 ⊆ 已记账是铁律；内部状态记账是浪费 |
| session 是插件吗 | 内核件，不可拔 | 不变量的载体；拔了崩溃恢复/回放全塌。开放性靠事件类型注册保留 |
| 翻译官给谁服务 | 机器对机器 | 人看的排版是 UI 投影；codec 的职责是无损持久化 + 版本 |
| Fork 要不要 | 砍 | Create(seed) 已覆盖其本质；Fork 是便利函数，需要时纯增量加 |
| 存活名单独立吗 | 并入算历史 | 它是投影的内部状态，不是独立能力（评审质疑成立） |
| 为什么必须 JSON | 无损 + 可版本化 | 结构体直接存绑死了内存布局；JSON 让旧账永远可读 |
| Session 依赖 core 吗 | **零依赖** | 记账要广播，但不拿整个 App（不回指整栋楼）——session 自定义一方法接口 `Broadcaster`，构造只收它；App 隐式满足。session 连 import core 都不需要，组装在 main 完成 |
| Message 属于谁 | 中立词汇包 chat | session 与 llm 共用公共词汇，避免前向引用（外部审核修正） |

## 验收测试（写代码前先写这些）

1. 记账契约：非法 JSON / 编号跳变 / Replaces 指向不存在的编号 → 当场报错；
2. **回放确定性**：`Create(seed)` 重建 → `ModelHistory()` 与原账逐字节一致（做成可复用测试工具）；
3. 压缩：写入带 Replaces 的摘要后，`ModelHistory` 跳过被取代段、`Events` 全量不变；
4. 未知事件：未知必需 → 拒绝重建；未知 Ignorable → 保留且跳过；
5. 记账即广播：每次 Append 后 M1 的 Broadcast 收到一条；
6. 缓存正确性：取代发生 → 缓存作废重算；无取代 → 增量追加不重算；
7. 深拷贝验证：记账后改传入的原结构体，账内容不变；
8. **故障注入六类**：短写、ENOSPC、失败后重试、尾部撕裂、并发 Append、重开修复——任何失败后介质无半行污染，重开只修可证明未提交的尾巴；
9. 不投影的 Kind 不进 ModelHistory；连续 chunk 折叠为一条；
10. 全程 `go test -race` 通过。

## 给 M3/M4 的接口预留

- M3（tools）：拿窄写入口记 tool/call、tool/start、tool/result（codec 归 session，tools 不自己注册）；
- M4（loop）：拿窄写入口记轮/步/消息/chunk/投递/领出/快照，消费 `ModelHistory`；
- 未来插件：注册自家事件类型记账；压缩插件只写带 `Replaces` 的摘要事件。
