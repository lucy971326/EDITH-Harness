# subagents

> 用父 Session 委派、等待和控制子 Session。

```text
主 Session（第 0 层）
   └─ task.json（直属关系）
          ↓
      子 Session（第 1 层）
         └─ task.json（直属关系）
                ↓
            孙子 Session（第 2 层）

每层状态 / 结果从自己的会话投影
回报只写进直属父账本
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
- 最大深度固定为 2；第 2 层不能继续委派。
- `task.json` 不复制 depth、rootID、状态、结果或 delivered。
- 子账本是执行事实；父账本中的 MessageID 是回报已送达的证明。
- Stop 沿父子关系停止目标及全部后代；正常完成不停止孩子。
- 内存缓存可以丢，重启后必须能从账本恢复判断。
