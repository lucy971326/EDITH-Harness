package session

// 数据。账本里一句话的身份。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// 数据。粘贴进来的图。Data 是 base64。工作区路径不当 Media，以后走 read_image。
type Media struct {
	MIME string `json:"mime"`
	Data string `json:"data"`
}

// 数据。模型发出的工具调用。
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// 数据。一句消息里的一个有序内容块。
type Block struct {
	Kind  string    `json:"kind"`
	Text  string    `json:"text,omitempty"`
	Tool  *ToolCall `json:"tool,omitempty"`
	Media *Media    `json:"media,omitempty"`
	Error string    `json:"error,omitempty"`
}

// 数据。账本中的一个完整节点内容。
type Message struct {
	Role   Role    `json:"role"`
	Blocks []Block `json:"blocks"`
}

// 数据。Runner 交给 Session 的用户输入。
type UserMessage struct {
	Blocks []Block `json:"blocks"`
}
