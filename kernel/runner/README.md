# runner

拥有一轮执行的准入、准备、取消、输出与收尾。

```text
Start -> 占用 live -> 准备配置 / Agent -> Loop.Run
                                            |
                         Emit -> 落账 / 更新草稿 -> 发布事件
                   Checkpoint -> 消费 Steer / 协作输入
结束或取消 -> 保存结果 -> 释放 live -> 完成句柄
```

## 阅读顺序

- `run.go / types.go`：Runner、liveRun、启动与共同收尾。
- `steer.go / collaboration.go`：外部输入、停止与协作消息去重。
- `emit.go / snapshot.go`：草稿、耐久消息、事件与快照边界。
- `records.go / permission_context.go`：运行记录与模型权限说明。
- `compact.go`：历史压缩；`diff_tracker.go / diff_store.go`：净变化聚合、保存与撤销。

同一 Session 只有一个活 Run；准备期也受停止控制。完整消息先落账再发布；停止拒绝尚未落账的 Steer，已发出的工具调用必须补齐结果。

内存保存 liveRun；`runs.json` 与 `diffs/` 保存运行结果，不混入对话账本。重启将未完成轮次标记中断，不自动续跑；`Close` 取消并等待全部运行。
