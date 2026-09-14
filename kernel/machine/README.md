# machine

> “操作这台电脑”的公共契约。

当前契约提供目录与文件操作，并通过 `ProcessSystem` 提供长期进程能力；真正的本机实现位于 `plugins/machine/local`。

```text
工具 → machine.Machine / ProcessSystem → local 实现 → 本机文件 / 进程
```

这里只定义能力形状，不直接碰操作系统。
