package session

// Role 是账本里一句话的身份。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// Media 是粘贴进来的图。Data 是 base64。工作区路径不当 Media，以后走 read_image。
type Media struct {
	MIME string `json:"mime"`
	Data string `json:"data"`
}

// ToolCall 是模型发出的工具调用。
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// Block 是一句消息里的一个有序内容块。
type Block struct {
	Kind   string    `json:"kind"`
	Text   string    `json:"text,omitempty"`
	Tool   *ToolCall `json:"tool,omitempty"`
	Media  *Media    `json:"media,omitempty"`
	Replay []byte    `json:"replay,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// Message 是账本中的一个完整节点内容。
type Message struct {
	Role   Role    `json:"role"`
	Blocks []Block `json:"blocks"`
}

// UserMessage 是 Runner 交给 Session 的用户输入。
type UserMessage struct {
	Blocks []Block `json:"blocks"`
}
