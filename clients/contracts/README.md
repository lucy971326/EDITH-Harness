# contracts

> Client 眼中的 JSON-RPC 契约。

```text
appserver.ts  初始化、模型、Agent、Skill、命令等公共接口
harness.ts    Harness 会话与产品操作
run.ts        Snapshot、Run、Entry 和实时事件
```

- Go 与 TypeScript 契约手工同步。
- 这里仅描述数据形状，不发送请求，也不实现业务。
- `contracts-types.test.ts` 检查 Client 侧关键类型是否仍能正确使用。

修改 RPC 时：先改 Go 契约，再同步这里，最后运行 `npm --prefix clients run contracts:check`。
