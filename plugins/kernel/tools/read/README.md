# read

【它是什么】读取文本文件的 Tool 插件。

【提供能力】不注册整份服务。

【使用能力】Resolve `machine`、`tools`。

【填充插槽】向 `tools` 登记 `read`。

## 代码主干

```text
模型参数 Path
  → 相对路径从 Workspace 起算
  → machine.ReadFile
  → 保留开头，截断到 2000 行 / 50 KiB
  → 返回文本
```

`plugin.go` 负责登记；`read.go` 定义参数和执行逻辑。
