# write

【它是什么】完整覆写文本文件的 Tool 插件。

【提供能力】不注册整份服务。

【使用能力】Resolve `machine`、`tools`。

【填充插槽】向 `tools` 登记 `write`。

## 代码主干

```text
模型参数 Path + Content
  → 相对路径从 Workspace 起算
  → machine.WriteFile
  → 返回写入字节数
```

本机 machine 会创建缺失的父目录；已有文件会被整体替换。

`plugin.go` 负责登记；`write.go` 定义参数和执行逻辑。
