package session

import "time"

// 数据。一本会话在账本外的元数据。元数据文件是会话存在的依据。
type SessionMeta struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

// 数据。账本里一句话的身份。
type Role string

const (
	RoleUser          Role = "user"
	RoleAssistant     Role = "assistant"
	RoleTool          Role = "tool"
	RoleSystem        Role = "system"
	RoleCollaboration Role = "collaboration"
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

// 数据。一次工具调用交还给模型的结果。
type ToolResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"isError,omitempty"`
}

// 数据。一句消息里的一个有序内容块。
type Block struct {
	Kind   string      `json:"kind"`
	Text   string      `json:"text,omitempty"`
	Tool   *ToolCall   `json:"tool,omitempty"`
	Result *ToolResult `json:"result,omitempty"`
	Media  *Media      `json:"media,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// 数据。账本中的一个完整节点内容。
type Message struct {
	MessageID       string  `json:"messageID,omitempty"`
	SourceSessionID string  `json:"sourceSessionID,omitempty"`
	SourceRunID     string  `json:"sourceRunID,omitempty"`
	RunID           string  `json:"runID,omitempty"`
	Role            Role    `json:"role"`
	Blocks          []Block `json:"blocks"`
}

// 数据。当前分叉上一条已落账消息及其稳定位置。
type Entry struct {
	ID       string  `json:"id"`
	ParentID string  `json:"parentID,omitempty"`
	Seq      uint64  `json:"seq"`
	Message  Message `json:"message"`
}

// 数据。Runner 交给 Session 的用户输入。
type UserMessage struct {
	Blocks []Block `json:"blocks"`
}
