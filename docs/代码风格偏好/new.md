# Harness 代码组织偏好

> 目标：先看懂主干，再进入细节。

## 核心原则

```text
一个领域一个内聚服务
服务内部按职责分章节
只有真正可替换的实现才设计插槽
```

不要把所有代码塞进一个大文件，也不要把每个小能力都注册成独立 Service。

```text
Plugin   安装单位：组装、注册、清理
Service  使用单位：一个领域的完整能力
Slot     替换位置：让 Provider / Adapter 填充
```

## 字符串能力名不变

```go
app.RegisterService("agents", service)
service, err := core.Resolve[agents.Service](app, "agents")
```

不改成类型 Key，不用包装函数隐藏字符串。名字或类型对不上属于组装错误。

## 包内按章节组织

默认先在同一个 Go package 内拆文件：

```text
agents/
├─ plugin.go                    安装入口
├─ service.go                   对外能力
├─ types.go                     公共数据
├─ conversation.go              Conversation 契约
├─ runner.go                    Runner 插槽
├─ conversation_manager.go      核心结构体
├─ conversation_start.go        创建会话
├─ conversation_resume.go       恢复会话
├─ conversation_close.go        关闭会话
└─ conversation_scope.go        作用域与清理
```

```text
plugin.go          解析依赖、创建实现、注册、清理
service.go         对外接口，只说明“能做什么”
types.go           跨包、传输或落盘的数据
<role>.go          核心结构体和共享状态
<role>_<action>.go 一条完整业务流程
```

`service.go` 是菜单，不放状态、长流程和私有辅助函数。不同文件的方法通过相同 receiver 归属于同一个结构体。

## 命名必须准确

```text
Registry  登记、查找、重复检查、撤销
Store     拥有并读写一组数据
Manager   管理对象的完整生命周期
Runtime   执行、分发、取消或协调实时工作
Adapter   翻译两个既有协议
Plugin    安装和卸载
```

只登记 Conversation 可以叫 `registry`；还负责创建、恢复和关闭，就应叫 `conversationManager`。宁可名字稍长，也不要让读者猜。

## 什么时候拆子包

同时满足以下条件才增加目录层级：

1. 可以独立替换或独立演进。
2. 输入输出边界稳定清楚。
3. 不会制造循环依赖和大量转发代码。

```text
优先：领域包 → 章节文件 → 函数
谨慎：领域包 → 子包 → 章节文件 → 函数
```

不因为文件长、类型多或“以后也许有用”而拆包。

## 当前方向

```text
llm      一个 Service：Adapter 注册 + 模型目录 + 调用
tools    一个 Service：工具登记 + 可见性 + 执行流水线
session  一个 Store：会话生命周期 + 事件日志 + 消息投影
agents   一个 Service：Conversation 生命周期 + Runner 插槽
web      一个插件：按 chat / project / preset / events 拆文件
```

优先整理内部文件，不拆出一串新的 App Service。

## 生命周期与重构

- 所有登记都应返回名为 `unregister` 的幂等撤销函数。
- 插件启动失败时，App 自动逆序撤销已经完成的登记。
- 谁创建长期资源，谁负责关闭。
- 一次只整理一个领域，先保持行为不变，测试通过再继续。
- 不顺手增加功能、重做 Service 或改变字符串能力名。

## 写法

- 可读性优先，不炫技。
- `err := f()` 与 `if err != nil` 分成两行。
- 每包一个主要公开构造入口；构造只校验依赖和组装。
- 接口只为真实边界定义，不为未来预建。
- 不建立无明确所有者的 `utils.go`、`helpers.go`、`common.go`。

## 项目沟通话术

解释新概念时，一次只讲一个，不先平铺整套系统。先判断它是不是插件，并明确当前代码还是未来计划。

如果是插件，固定说明四件事：

```text
【它是什么】
一句话说明插件职责

【提供能力】
它给别人什么能力

【使用能力】
它领取哪些已有能力

【填充插槽】
它填充哪个插槽；没有就明确写“暂不填充插槽”
```

如果不是插件，固定说明三件事：

```text
【它是什么】
它属于什么概念

【它的作用】
它负责什么

【和插件的关系】
哪些插件产生、使用或依赖它
```

解释时遵守：先说人话，再说代码名；出现新黑话立即解释；不确定就查代码，不猜；不把未来设计说成已经存在。

最终标准：看目录和 `service.go` 就能说清领域职责；想了解某条流程时，可以直接找到对应文件。
