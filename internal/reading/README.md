# reading

当前本机用户共享的阅读位置。`store.go` 经 persist 原子保存每个会话最后已读结果的 Run 起始序号；并发确认只能前进。conversations 验证实际 Run，appserver 在成功保存后通知所有客户端。删除会话时一起清理。
