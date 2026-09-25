# backend

`Open` 是 Web 与 Desktop 共用的显式装配。先锁定 `~/.harness`，再创建服务、登记工具与方法；失败或退出时按相反顺序关闭，最后释放文件锁。

```text
LockRoot → Files / Machine / 工具与领域服务 → appserver
Close    ← 逆序释放                         ←
```

这里不持有会话业务状态；运行入口分别在 `cmd/harness` 和 `cmd/harness-desktop`。
