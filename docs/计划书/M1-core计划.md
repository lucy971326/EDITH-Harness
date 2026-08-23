# M1 计划书 —— core 包

> 状态：已定稿（经第一性原理评审，砍掉启动器，四个功能）。
> 本文是施工依据，写代码前先读一遍，改本文 = 改设计。

## 职责（一句话）

**让一堆互不认识的代码，能在同一个进程里：找到彼此、说话、互不打扰。**

core 不懂任何 agent 知识——不知道模型、工具、对话为何物。纯协作基础设施。

## 不做什么（边界）

- ❌ 不管对话与记账（M2）
- ❌ 不管模型与工具（M3）
- ❌ 不管进程与文件（M5）
- ❌ 不读配置文件（M6），M1 阶段插件用代码手动安装
- ❌ 不做插件卸载/热加载（进程重启即干净）
- ❌ 不做拓扑排序（安装顺序即启动顺序）

---

## 四个功能

### ① 服务表（放东西 / 拿东西）

- `map[能力名]any`，**键是能力名，不是插件名**（取用者只关心"我要 tools"，不关心谁提供的——换实现不伤人的前提）；
- **值是"结构体挂方法"的能力对象，不是回调函数**（回调藏不住状态）；
- 取用走泛型函数获得类型安全：`tools.Get(app) → *tools.Registry`；
- 启动阶段缺服务 → 返回 error（调用方处置）；服务类型不匹配 → panic（框架编程错误）。
- **运行期不再碰服务表**：Start 一次解析成类型化字段，插件不持有整个 App——用 API 形状引导，不靠文档自觉。

```go
app.RegisterService("tools", &tools.Registry{})
reg, err := tools.Resolve[*tools.Registry](app)  // Start 内解析；缺 → err；类型错 → panic
```

### ② 事件系统（说事情 / 插一手）

四种分发方式，**各自失败语义，不许一刀切 recover**：

| 方式 | 语义 | 失败语义 |
|---|---|---|
| `Broadcast` | 通知，不等待 | 观察者 panic 隔离为日志，不影响主流程 |
| `RunChain` | 中间件链：改内容 / 拦截 | 错误向上传；安全场景失败即拒 |
| `RunConcurrently` | 全部并发，等全部完成 | 聚合失败并返回 error |
| `RunSequentially` | 按注册顺序执行 | 出错即中断并返回 error |

工程约束：

- **Broadcast 是同步分发**（按注册顺序直接调用），不逐事件起 goroutine——防流式输出触发 goroutine 风暴；要异步的监听器自己声明；
- **channel 不用于总线分发**：channel 是管道（一对一运输），总线是规章（谁听、什么顺序、错了怎么办）。channel 照常用于 loop 唤醒、模型流式等**能力内部**；
- `RunChain` 监听器签名：`func(payload P, next func() R) R`——调 `next()` 放行，不调即拦截。
- **Broadcast 回调内禁止同步 Append**（写锁未放则死锁、放了则重入插账）——需要记账就转异步投递（五轮复核）。

### ③ 生死收摊（App.Close + 作用域随主消亡）

不为热加载，为**进程内生死**：

- agent 关闭 → 它的小 map、它挂的监听器整批消失（防泄漏、防死 agent 遮蔽全局）；
- 服务器版进程跑数月、测试每用例建一个 App，都靠这个收尾；
- 原则：**谁创建谁收摊**（`app.OnCleanup(fn)`，Close 时逆序执行）。

### ④ 作用域（公共的 vs 私有的）

- 全局一张大 map + 每个 agent 一张小 map；
- 查找：**先小 map，查不到落大 map**；同名时小 map 赢（遮蔽）；
- 限制（restrict）只裁从大 map 继承来的，裁不到自己小 map 里的；
- agent 死了 → 它的小 map 整张扔掉（与③咬合）。

```go
tools.AddGlobal("bash", 普通bash)
tools.AddFor(agentA, "bash", 沙箱bash)
tools.Lookup("bash", agentA)  // → 沙箱bash（小 map 命中）
tools.Lookup("bash", agentB)  // → 普通bash（落到大 map）
```

---

## 文件与体量

```
core/
  app.go     App：服务表 + 事件订阅表 + OnCleanup/Close + ForAgent 视图
  events.go  Subscribe + 四种分发（各自失败语义）
  scope.go   大 map + 每 agent 小 map（遮蔽 / 限制 / 随主扔掉）
  types.go   数据契约（Listener、事件负载等）
```

四个文件，估计 ~300 行。插件接口只有三行（`Name() + Start(app)`），放 app.go，不单开文件。

**安装即启动**：main 里按顺序 for 循环调 `plugin.Start(app)`，无宿主、无排序器。

---

## 裁决记录（为什么是这样）

| 问题 | 裁决 | 理由 |
|---|---|---|
| 服务表的键 | 能力名，非插件名 | 一插件多能力；取用者不应知道提供方是谁 |
| 服务表的值 | 能力对象（结构体+方法） | 回调藏不住状态，结构体可测可持依赖 |
| 事件分发用 channel？ | 否 | channel=一对一管道，总线=一对多规章；channel 用在能力内部 |
| 统一 recover？ | 否 | 观察隔离 / 链上传导 / 屏障聚合，三种语义分开 |
| 插件卸载清理？ | 改名收摊 | 不热加载；但 agent 生死和测试收尾必须干净 |
| 拓扑排序启动？ | 砍 | 手写组装自己排顺序；将来 YAML 组装再加（纯增量） |
| 运行期能碰服务表吗 | 不能 | Start 一次解析成字段、不持有整个 App；API 引导而非文档纪律（外部审核强化） |
| 缺服务怎么报错 | 启动缺 → error；类型错 → panic | 缺服务是配置问题可处置；类型不匹配是编程错误 |
| 启动中途失败 | 逆序回滚已启动插件 | 谁创建谁收摊从第一天成立 |

---

## 验收测试（写代码前先写这些）

1. 四种分发各自的失败语义（panic 隔离**只在** Broadcast 生效；Chain 错误上抛；Concurrently 聚合；Sequentially 中断）；
2. Broadcast 同步执行、零新增 goroutine（goroutine 计数断言）；
3. RunChain：中间件改写内容生效、不调 next 即短路；
4. 服务表：注册 / 类型安全取用 / 取不到当场报错；
5. 作用域：遮蔽、restrict 只裁继承层、agent 关闭后小 map 与其监听器消失；
6. OnCleanup 逆序执行；
7. 启动第 N 个插件失败 → 已启动的逆序回滚（cleanup 全部执行）；
8. Broadcast 回调内同步 Append 被拒绝；
9. 全程 `go test -race` 通过。

---

## 给下游的接口预留（终审修正）

- session（M2）用：`Broadcast`（记账后通知 UI）；Journal（原子提交边界）由持久化插件登记，session 同步领取；
- loop（M4）用：`RunChain`（pre-step 检查站）；
- 投影缓存是 session 自己的内部状态，不经过 core。
