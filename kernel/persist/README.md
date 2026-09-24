# persist

固定的本机文件读写服务，通常以 `~/.harness` 为根目录。

```text
各领域的 Store -> Files.Scope(子目录)
                    +-> Read / List / Remove
                    +-> Write  临时文件 + 原子替换
                    +-> Append 追加 + 同步
```

源码集中在 `files.go`。各 Scope 共享文件操作锁；不提供跨文件事务，读改写的业务锁由所属领域负责。

不认识 Session、Agent 或 Run 格式。格式、校验和恢复归使用者；不做旧格式迁移或存储介质适配层。
