# contracts

手写 TypeScript 契约，与 appserver 的 Go 契约人工对齐。

```text
Go 接口 <-> 人工核对字段 / 可选性 / 方法名 <-> TS 类型
                                                    -> Client
```

- `appserver.ts`：公共查询、文件、终端与 Hooks 等接口。
- `harness.ts`：会话和子任务操作。
- `run.ts`：快照、账本投影和运行事件。
- `approvals.ts`：权限模式、审批请求与审核设置。
- `contracts-types.test.ts`：类型用法检查。

不发送请求、不实现业务、不生成 Go 类型。`npm --prefix clients run contracts:check` 只检查 TS，不能代替两端契约核对。
