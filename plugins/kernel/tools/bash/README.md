# bash

【它是什么】执行 Bash 命令的 Tool 插件。

【提供能力】不注册整份服务。

【使用能力】Resolve `machine`、`tools`。

【填充插槽】向 `tools` 登记 `bash`。

## 代码主干

```text
模型参数 Command
  → machine.Run(Workspace, ["bash", "-c", Command])
  → 合并标注 stdout / stderr
  → 保留结尾并截断
  → 返回普通结果或可见错误
```

非零退出会作为 Tool 错误交还模型；取消会直接终止本轮调用。

`plugin.go` 负责登记；`bash.go` 定义参数和执行逻辑。
