package agents

import (
	"fmt"

	"harness/core"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
)

func (m *conversationManager) mountScope(sessionID string, root string, allowed []string) (*core.App, error) {
	scope := m.app.ForChild(sessionID)
	err := m.environment.Mount(scope, root)
	if err != nil {
		scope.Close()
		return nil, fmt.Errorf("会话 %s 挂载执行环境失败：%w", sessionID, err)
	}
	err = m.tools.SetAllowedForScope(sessionID, allowed)
	if err != nil {
		scope.Close()
		return nil, err
	}
	return scope, nil
}

func (m *conversationManager) prepare(runner Runner, project projects.Project, preset presets.Revision, model llm.Selection, scope *core.App, book *session.Session, recover bool) (PreparedConversation, error) {
	sessionID := book.ID()
	input := RunInput{
		SessionID: sessionID,
		Project:   project,
		Preset:    preset,
		Model:     model,
		Scope:     scope,
		Book:      book,
		Recover:   recover,
		Close: func() error {
			return m.finish(sessionID, scope)
		},
		ReportError: func(cause error) {
			if cause == nil {
				return
			}
			m.app.Broadcast(EventConversationError, ConversationError{
				SessionID: sessionID,
				Message:   cause.Error(),
			})
		},
	}
	return runner.Prepare(input)
}
