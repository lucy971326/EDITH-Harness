# subagents

管理父子会话关系、委派、等待、回报与整族停止；执行仍交给同一个 Runner。

```text
主 Session (0) -> 子 Session (1) -> 孙 Session (2)
                     |
               Runner 执行 -> 完成回报 -> 直属父账本
```

## 从哪里读

- `subagents.go / types.go`：服务生命周期、共享状态与对外数据。
- `spawn.go / send.go / turn.go`：创建孩子、继续任务与启动轮次。
- `access.go / family.go / stop.go`：归属、深度与停止代次。
- `wait.go / projection.go`：等待通知、组合任务视图。
- `notifications.go / store.go`：回报投递与任务关系文件。

任务关系存在 `subagents/tasks/<id>.json`；状态和结果来自子会话，父账本中的 MessageID 是投递确认。不重复保存 depth、结果或 delivered。

最大深度为 2，父子共享工作区。Stop 取消目标及全部后代，正常完成不停止孩子；父闲时不因回报自动启动。重启恢复关系但不续跑；Close 解除订阅、取消并等待后台工作。
