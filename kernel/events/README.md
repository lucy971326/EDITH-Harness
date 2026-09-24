# events

按 Go 类型区分事件的进程内同步通知。

```text
Subscribe[T] -> 回调列表
Publish[T]   -> 按登记顺序调用 -> 汇总错误
取消订阅     -> 移除回调（幂等）
```

源码集中在 `registry.go`。Runner 发布运行事件，appserver 与 Subagents 等监听；回调在登记处锁外执行。

不保存、缓冲或重放事件。监听者必须及时返回；网络排队和慢连接处理属于 appserver。
