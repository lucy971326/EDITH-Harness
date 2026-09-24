# appserver

网络入口：校验请求、调用领域服务、返回结果与通知。

```text
Client -> WebSocket -> clientconn -> rpc.Registry -> 具名 Handler
                                                   +-> conversations
                                                   +-> 公共服务
```

## 从哪里读

- `server.go / listener.go / lifecycle.go / websocket.go`：组装、监听、接入与关闭。
- `harness.go / harness_types.go`：会话方法与契约；`harness_run.go`：运行订阅；`harness_subagents.go`：子任务接口。
- `agents.go / models.go / skills.go / commands.go`：公共能力接口。
- `approvals.go / approval_types.go / hooks.go`：审批与 Hook 配置。
- `filesystem.go / command_exec.go / workspace.go`：文件、用户终端和目录选择。

```text
internal/rpc             方法校验与同 Session 写请求排序
internal/clientconn      一条连接、通知队列与订阅清理
internal/workspacepicker 原生目录选择
```

同会话写请求排队，不同会话并行，Stop 独立执行。订阅先接事件再取快照，响应写出后才开放通知；慢连接断开，不阻塞 Runner。断线只清理连接资源，不停止已接受的 Run。
