# Dock 计划书

> 状态：已完成。  
> 日期：2026-09-02

## 目标

在 Chat 输入框上方增加 Dock：Todo、Goal、Subagent 等插件可以登记一项持续状态，并在状态变化时实时更新页面。

```text
业务插件改变状态
      ↓
发布 DockChanged
      ↓
Chat 调用 Dock.Render()
      ↓
现有 SSE 直接发送 Templ HTML
      ↓
HTMX sse-swap 替换对应内容
```

Dock 只是 Chat 的 `list` 插槽，不保存业务状态，也不认识 Todo、Goal、Subagent。

## 边界

```text
Chat               提供 Dock UI 插槽：登记处、折叠外壳、排序、SSE 转发
填充 Dock 的插件    拥有或读取自己的状态，登记条目并提供 Render()
events             传递状态已变化的进程内通知
浏览器             保存当前页面的展开 / 折叠状态
Session            只保存对话，不保存 Dock 状态
```

是否落盘由业务插件决定，不属于 Dock 契约。本阶段不实现通用 Storage。

## 契约

```go
// 数据。一个已登记 Dock 条目的静态身份。
type DockDefinition struct {
	ID    string
	Name  string
	Order int
}

// 数据。渲染 Dock 所需的当前会话事实。
type DockContext struct {
	SessionID string
	Workspace string
}

// 契约。一个可填入 Chat 输入框上方的持续状态条目。
type Dock interface {
	Definition() DockDefinition
	Render(DockContext) (templ.Component, error)
}

// 数据。一个 Dock 条目的状态已经变化。
type DockChanged struct {
	SessionID string
	DockID    string
}
```

`chat.Service` 增加：

```go
RegisterDock(Dock) error
Docks() []DockDefinition
Dock(id string) (Dock, bool)
```

规则：

- `ID`、`Name` 必填，拒绝重复 `ID`。
- `ID` 只允许小写字母、数字和 `-`，并以小写字母开头；它同时用于 DOM ID 和 SSE 事件名。
- `Docks()` 按 `Order`、再按 `ID` 稳定排序。
- 不提供图标字段；Chat 使用浏览器原生的折叠标记。
- 没有插件填充 Dock 时，页面不渲染 Dock 区域。
- 一个已填入的条目如何表达“暂无内容”，由提供它的插件决定。

## 页面与 SSE

Chat 固定绘制每个 Dock 的 `<details>` 和标题，插件只绘制内容。条目首次出现时默认折叠；SSE 只替换内部内容，因此用户当前的展开状态不会丢失。

```html
<details>
  <summary>Todo</summary>
  <div sse-swap="dock-todo">
    <!-- Todo.Render() 生成的 HTML -->
  </div>
</details>
```

一个页面仍然只有一条 SSE 连接。把现有 `hx-ext="sse"` 和 `sse-connect` 放到消息区、Dock、输入框共同的外层元素上；这只改变 HTML 属性位置，不改变页面布局。

```html
<div hx-ext="sse" sse-connect="/chat/{sessionID}/events">
  <section>消息区</section>
  <section>Dock</section>
  <form>输入框</form>
</div>
```

SSE 消息分为：

```text
event: run            data: Runner JSON
event: dock-todo      data: Todo 的最新 HTML
event: dock-goal      data: Goal 的最新 HTML
```

这里的 `dock-{ID}` 只是浏览器传输地址，不是新的业务事件类型。业务插件仍然只发布强类型的 `DockChanged`。

## 更新与恢复

```text
插件提交状态成功
  → events.Publish(DockChanged{SessionID, DockID})
  → Chat 的回调只把通知放入该 Session 的页面队列
  → 对应 SSE Handler 调用 Dock.Render()
  → 写出 event: dock-{DockID} 和 Templ HTML
```

- 发布回调不能等待浏览器，也不能在业务写入完成前通知。
- 未登记的 `DockID` 不发送到浏览器。
- 一个 Dock 渲染失败不能关闭 SSE，也不能影响聊天消息；该条目显示简短加载错误，后续变化可以再次渲染。
- SSE 写 HTML 时必须正确处理多行 `data:`，不能假设 Templ 输出永远只有一行。
- 每次 SSE 首次连接或重连，在完成订阅后依次渲染并发送全部已登记 Dock 的当前内容；不增加 `snapshot`、`seq` 或额外恢复协议。
- 慢客户端继续沿用现有规则：队列满就断开，重连后重新取得当前 Dock 内容。

## 演示插件

新增独立动态 Demo 插件验证真实边界：

- Resolve `chat.Service`、`events`，登记一个 Dock。
- 使用内存保存按 Session 区分的演示计数。
- 状态变化后发布 `DockChanged`，验证页面收到新的 Templ HTML。
- 不写 Session、不使用 Storage、不启动后台任务。
- 只用于自动化测试，不安装到正常的 `cmd/harness`。

## 验收

- 登记：合法条目、非法 ID、空字段、重复 ID、`Order / ID` 排序。
- 页面：没有登记项时没有 Dock；有多个登记项时顺序正确且默认折叠。
- 实时：一个 Dock 变化只产生对应的 `dock-{ID}` HTML 事件，`run` 事件不受影响。
- 状态：更新内部内容不会替换 `<details>`，展开 / 折叠状态保持。
- 恢复：首次连接和重连都会得到所有 Dock 的当前内容。
- 隔离：事件只进入对应 Session 的 SSE 连接。
- 错误：未知 Dock 不发送；单个 Render 失败不关闭连接。
- 协议：多行 HTML 能通过 SSE 完整还原。
- Demo：跑通“状态变化 → events → Chat → SSE HTML → HTMX 替换”。
- 完成后运行 Templ 生成、Tailwind 构建、Go 全量测试、Chat race 测试、`go vet`、Node 测试和 `git diff --check`。

## 不做

- 通用 Storage 服务。
- Todo、Goal、Subagent 的真实业务。
- 图标系统。
- Dock 片段 GET 路由。
- Dock 专用 JavaScript。
- 第二条 SSE、WebSocket、JSON reducer。
- `snapshot`、`seq`、动态插件或运行时热插拔。
- Workspace / 全局范围的 Dock 广播；首版只按 Session 更新。
