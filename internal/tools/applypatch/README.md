# apply_patch

把模型提交的补丁转成有版本保护的文件修改。

```text
补丁 -> 解析 / 上下文匹配 -> 计算全部修改 -> 必要时审批
  -> machine.AgentApplyChanges -> 已提交变化 -> Runner Diff
```

- `apply_patch.go`：`New` 与工具执行主线。
- `parser.go / types.go`：补丁语法和操作形状。
- `file_update.go / seek_sequence.go / text_file.go`：上下文定位、内容计算和换行处理。

通过 FileSystem 预读，通过 AgentFiles 携带可信 Policy 提交。审批展示真实开放目录，等待期间不持文件锁；获批后仍检查版本。中途失败返回已提交前缀，不假装全部成功；本包不保存 Run Diff 或账本。
