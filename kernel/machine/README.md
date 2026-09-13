# machine

> “操作这台电脑”的公共契约。

当前契约提供目录读取、文件读取/写入/编辑和命令执行等能力；真正的本机实现位于 `plugins/machine/local`。

```text
工具 → machine.Machine 接口 → local 实现 → 本机文件 / 进程
```

这里只定义能力形状，不直接碰操作系统。
