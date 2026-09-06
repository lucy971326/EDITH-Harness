package subagents

import delegation "harness/kernel/subagents"

// 数据。查询在保存或读取出错时仍返回可见记录，同时明确报告错误。
type listResult struct {
	Tasks []delegation.TaskView `json:"tasks"`
	Error string                `json:"error,omitempty"`
}

// 数据。查询可用设置的空参数。
type optionsArgs struct{}

// 数据。模型只能指定委派说明与可选设置；身份、工作区由程序提供。
type spawnArgs struct {
	Description     string `json:"description" jsonschema:"minLength=1,description=Self-contained instructions for the child. Parent history is not copied."`
	AgentID         string `json:"agentID,omitempty" jsonschema:"minLength=1,description=Agent ID from subagent_options. Omit to inherit the parent run setting."`
	Model           string `json:"model,omitempty" jsonschema:"minLength=1,description=Model ID from subagent_options. Omit to inherit."`
	ReasoningEffort string `json:"reasoningEffort,omitempty" jsonschema:"minLength=1,description=Supported reasoning effort. Omit to inherit; incompatible inherited effort is an error."`
}

// 数据。追加要求只能传文字，不能更改孩子的运行设置。
type sendArgs struct {
	TaskID string `json:"taskID" jsonschema:"minLength=1"`
	Text   string `json:"text" jsonschema:"minLength=1,description=Additional instructions for this child."`
}

// 数据。查询全部孩子或单个孩子。
type listArgs struct {
	TaskID string `json:"taskID,omitempty" jsonschema:"minLength=1"`
}

// 数据。等待任一孩子的新完成结果；已见通知 ID 可避免重复报告。
type waitArgs struct {
	TaskIDs             []string `json:"taskIDs" jsonschema:"minItems=1"`
	SeenNotificationIDs []string `json:"seenNotificationIDs,omitempty"`
	TimeoutSeconds      *int     `json:"timeoutSeconds,omitempty" jsonschema:"minimum=0,maximum=60,description=Wait duration in seconds. Default 60; 0 returns immediately. Timeout does not stop the child."`
}

// 数据。停止单个孩子的参数。
type stopArgs struct {
	TaskID string `json:"taskID" jsonschema:"minLength=1"`
}

// 数据。停止请求已被接受，不代表运行已经收尾。
type stopResult struct {
	TaskID        string `json:"taskID"`
	StopRequested bool   `json:"stopRequested"`
}
