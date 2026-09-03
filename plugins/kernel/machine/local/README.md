# machine-local

【它是什么】本机版 `machine` 提供者插件。

【提供能力】注册整份服务 `machine`，实现读文件、写文件、运行进程。

【使用能力】无。

【填充插槽】不填。

## 代码主干

```text
Start
  → 查找 bash（Windows 优先 Git Bash）
  → RegisterService("machine", local)

ReadFile / WriteFile  → 本机文件系统
Run(dir, argv)        → 在 dir 下启动进程
```

`bash` 参数会替换为找到的真实路径；插件不限制文件访问范围。

`plugin.go` 负责装卸；`local.go` 负责实际操作。
