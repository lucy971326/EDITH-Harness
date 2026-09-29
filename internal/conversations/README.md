# conversations

编排创建、发送、设置、停止、分叉和命令等会话操作。

```text
appserver -> Service
              +-> Send 闲时 -> Runner.Start
              +-> Send 忙时 -> Runner.Steer
              +-> 停止     -> Subagents.StopFamily
              +-> 会话资料 -> Session / Settings
```

- `service.go`：主流程与同会话操作锁；等待 Steer 落账前释放锁，Stop 不取操作锁。
- `reading.go`：组合轻量 Run 状态与独立阅读位置，验证实际已完成的 Run 后确认已读。
- `subagents.go`：子任务页面的查询、发送与设置。
- `types.go / errors.go`：操作输入、快照与可识别错误。

单会话通过 Store.Meta 按 ID 读取，不扫描其他会话；缺失与损坏保持不同错误分类。

Runner 拥有实际执行，Session 拥有账本。这里不处理网络协议或文件格式；公共模型、Agent、Skill 列表由 appserver 直接调用所属服务。
