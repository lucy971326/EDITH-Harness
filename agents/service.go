package agents

import (
	"harness/core"
	"harness/session"
)

// Service 是其他插件管理运行中会话的公共入口。
type Service interface {
	StartSession(input StartInput, seed ...session.Event) (Conversation, error)
	ResumeSession(sessionID string) (Conversation, error)
	OpenSession(sessionID string) (Conversation, error)
	GetSession(sessionID string) (Conversation, error)
	CloseSession(sessionID string) error
	RegisterRunner(runner Runner) (func(), error)
}

// Get 从 App 取 Agent 管理入口。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "agents")
}
