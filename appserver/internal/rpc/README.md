# rpc

```text
JSON 参数 → Schema 校验 → 到达即排队 → Go Handler → 输出校验 → JSON 结果
```

- `registry.go`：方法名找到处理函数。
- `method.go`：先准备调用，再等待完整结果。
- `scheduler.go`：只给同一 Session 的写请求按到达顺序排队。
- `error.go`：稳定错误分类。

这里不懂 Harness 业务，也不保存 Session。
