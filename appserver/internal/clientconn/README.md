# clientconn

一条 Client 连接的生命周期，不拥有会话业务。

```text
WebSocket -> 初始化 -> 收请求 / 回响应
                      +-> 通知队列
断线                  -> 取消请求 / 清理订阅与连接资源
```

`connection.go` 管连接、请求分发与通知发送，`requests.go` 管请求准入和关闭等待，`notifications.go` 管订阅响应屏障。

请求在接收阶段完成校验与必要排队，执行异步进行，Stop 不被普通写请求挡住。已接受的 Run 生命周期独立于连接。
