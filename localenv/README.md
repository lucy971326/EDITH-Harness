# localenv

一句话：**本机执行环境提供者**，把一个项目目录挂进某段会话。

```text
agents 创建会话作用域
          │ Mount(root)
          ▼
       localenv
          │
          ├─ files
          ├─ process
          └─ shell
```

- 提供能力：`"environment"`
- 真正的 `files/process/shell` 只存在于某个会话作用域，不是全局能力。
- 会话关闭时，它负责收掉该会话启动的进程。
- 先读：`plugin.go` → `files.go` → `process.go`。
