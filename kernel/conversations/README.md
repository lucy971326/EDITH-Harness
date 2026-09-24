# conversations

编排创建、发送、设置、停止、分叉和命令等会话操作。

```text
appserver -> Service
              +-> 闲时发送 -> Runner.Start
              +-> 忙时输入 -> Runner.Steer
              +-> 停止     -> Subagents.StopFamily
              +-> 会话资料 -> Session / Settings
```

- `service.go`：主流程与同会话操作锁；等待 Steer 落账前释放锁，Stop 不取操作锁。
- `subagents.go`：子任务页面的查询、发送与设置。
- `types.go / errors.go`：操作输入、快照与可识别错误。

Runner 拥有实际执行，Session 拥有账本。这里不处理网络协议或文件格式；公共模型、Agent、Skill 列表由 appserver 直接调用所属服务。
