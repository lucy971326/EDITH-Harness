# rpc

类型化方法登记、校验与请求排序。

```text
JSON -> 输入校验 -> 必要时按 Session 排队
  -> Go Handler -> 输出校验 -> JSON
```

- `registry.go`：登记方法，按名称查找。
- `method.go`：准备与执行一次调用。
- `scheduler.go`：按接收顺序执行同 Session 写请求。
- `error.go`：协议边界的错误分类。

不保存 Session，也不判断 Start / Steer / Stop；是否排队由方法登记决定。
