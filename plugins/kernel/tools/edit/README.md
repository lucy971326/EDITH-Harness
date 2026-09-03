# edit

【它是什么】精确替换文件中一段文本的 Tool 插件。

【提供能力】不注册整份服务。

【使用能力】Resolve `machine`、`tools`。

【填充插槽】向 `tools` 登记 `edit`。

## 代码主干

```text
模型参数 Path + OldText + NewText
  → machine.ReadFile
  → 确认 OldText 恰好出现一次
  → 替换一次
  → machine.WriteFile
```

零次或多次匹配都不会修改文件；写入前会再次检查取消状态。

`plugin.go` 负责登记；`edit.go` 定义参数和执行逻辑。
