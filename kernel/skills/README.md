# skills

登记 Skill 来源，按工作区合并名称、描述和位置。

```text
builtin / filesystem -> Register Provider
工作区              -> List -> 校验 / 稳定排序 -> agents.Prepare
```

`types.go` 定义 Skill 摘要与 Provider，`registry.go` 实现合并。每轮动态发现可见 Skills，不复制进 Agent 设置。

跨 Provider 同名报错；文件系统来源内部的覆盖顺序由该 Provider 决定。登记处不注入正文、不执行 Skill，Agent 按位置读取具体说明。
