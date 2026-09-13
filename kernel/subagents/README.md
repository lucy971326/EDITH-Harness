# subagents

> 用父 Session 委派、等待和控制子 Session。

```text
父 Session
   └─ task.json（只记关系）
          ↓
      子 Session（完整普通会话）
          ↓
状态 / 结果从子会话投影
          ↓
回报写进父账本
```

## 从哪里读

```text
types.go          对外输入、结果与 TaskView
subagents.go      服务组成、生命周期和共享状态
spawn.go          创建子任务
send.go           向子任务追加指令
wait.go           等待完成通知
stop.go           停止任务族
projection.go     子会话事实 → TaskView
notifications.go  将完成回报可靠送进父账本
store.go          task.json 关系文件
```

## 铁律

- 子 Session 与主 Session 平级，格式完全一致。
- `task.json` 不复制状态、结果或 delivered。
- 子账本是执行事实；父账本中的 MessageID 是回报已送达的证明。
- 内存缓存可以丢，重启后必须能从账本恢复判断。
