# subagent tools

把父子会话协作能力暴露为模型工具。

```text
options / spawn / send / list / wait / stop
  -> internal/subagents -> Runner -> 子会话
```

- `construct.go`：`Register` 登记六个带 `subagent_` 前缀的工具。
- `types.go`：模型参数；`tools.go`：工具说明与调用转发。

Session / Run / ToolCall 身份由程序传入，模型不能冒充父任务。关系、深度、通知和停止规则归 Subagents；本包不保存第二份任务状态。
