package agents

import (
	"harness/llm"
	"harness/session"
)

// EventConversationError 是会话运行失败时的实时事件名。
const EventConversationError = "agents/conversation-error"

// Conversation 是一段运行中会话对外开放的操作。
type Conversation interface {
	SessionID() string
	State() string
	WaitIdle()
	Cancel()
	SubmitFollowup(text string) error
	Steer(text string) error
	SelectModel(selection llm.Selection) error
	Book() *session.Session
	Close() error
}

// PreparedConversation 是已恢复、尚未开始搬运消息的会话。
type PreparedConversation interface {
	Conversation
	Start()
}

// ConversationError 是会话运行中的实时故障通知；它不写进账本。
type ConversationError struct {
	SessionID string // 哪段会话
	Message   string // 给界面显示的失败原因
}
