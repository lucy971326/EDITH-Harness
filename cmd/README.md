# cmd

可执行程序入口。当前只有 [`harness/`](harness/README.md)，负责装配和进程生命周期。

```text
cmd/harness/main.go
  -> 构造服务 -> 登记扩展 -> 绑定 RPC -> 启动监听
  -> 退出时逆序关闭
```

业务流程与具体实现在 `internal` 各领域；入口只接线。
