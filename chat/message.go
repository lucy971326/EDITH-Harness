// Package chat 是对话的公共词汇：session 和 llm 都说这一套，
// 谁也不借用谁还没写出来的类型。
package chat

import "encoding/json"

// Message 是一条对话消息：模型看到的最小单位。
// 文字和工具调用可以在同一条助手消息里；工具结果单独一条。
type Message struct {
	Role        string       // "system" / "user" / "assistant" / "tool"
	Text        string       // 说的文字
	Interrupted bool         // 助手消息专用：true = 被打断的半句（界面上可以画成"未说完"）
	Media       []Attachment // 夹带的图片/音频（多模态输入），纯文字时为空
	Calls       []ToolCall   // 发起的工具调用（助手消息可带多个）
	Result      *ToolResult  // 带回的工具结果（tool 消息专用）
}

// Attachment 是消息里夹带的一份非文字内容。
// 翻译官（M3 适配器）负责把它转成各家 API 要的样子：URL 或 base64。
type Attachment struct {
	Kind string // "image" / "audio"
	URL  string // 从哪来：http 地址或本地文件路径，与 Data 二选一
	Data []byte // 或者直接给内容字节，与 URL 二选一
	Mime string // 内容类型，如 "image/png"；不知道可空着
}

// ToolCall 是模型发起的一次工具调用。
type ToolCall struct {
	ID       string          // 这次调用的编号，结果凭它对账
	Name     string          // 工具名
	Argument json.RawMessage // 参数原文（模型给的 JSON，原样保留）
}

// ToolResult 是一次工具调用的回话。
type ToolResult struct {
	CallID string // 对应哪次调用
	Output string // 工具吐回的内容
	Status string // "success" / "failed" / "skipped"
}

// Usage 是一次模型请求的用量统计。
type Usage struct {
	InputTokens  int
	OutputTokens int
}
