# persist

固定的本机文件读写服务，通常以 `~/.harness` 为根目录。

```text
各领域的 Store -> Files.Scope(子目录)
                    +-> Read / List / Remove
                    +-> Write  临时文件 + 原子替换
                    +-> Append 追加 + 同步
```

`files.go` 负责文件操作；`lock*.go` 在后台启动前独占数据目录。各 Scope 共享进程内文件操作锁；不提供跨文件事务，读改写的业务锁由所属领域负责。

不认识 Session、Agent 或 Run 格式。格式、校验和恢复归使用者；不做旧格式迁移或存储介质适配层。
