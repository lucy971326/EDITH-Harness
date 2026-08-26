package agents

import (
	"fmt"

	"harness/llm"
	"harness/presets"
	"harness/projects"
)

// ResumeSession 按账本封面恢复旧会话，继续使用当时锁定的模式版本和项目根目录。
func (m *conversationManager) ResumeSession(sessionID string) (Conversation, error) {
	book, err := m.books.Open(sessionID)
	if err != nil {
		return nil, err
	}
	header := book.Header()
	preset, err := m.presets.GetRevision(header.PresetID, header.PresetRevision)
	if err != nil {
		_ = m.books.Release(sessionID)
		return nil, fmt.Errorf("会话 %s 的 Agent 模式版本不存在：%w", sessionID, err)
	}
	err = presets.Validate(preset)
	if err != nil {
		_ = m.books.Release(sessionID)
		return nil, err
	}
	err = m.tools.CheckAllowed(preset.Tools)
	if err != nil {
		_ = m.books.Release(sessionID)
		return nil, fmt.Errorf("会话 %s 的历史工具不完整：%w", sessionID, err)
	}
	runner, err := m.reserve(sessionID)
	if err != nil {
		_ = m.books.Release(sessionID)
		return nil, err
	}
	scope, err := m.mountScope(sessionID, header.ProjectRoot, preset.Tools)
	if err != nil {
		_ = m.books.Release(sessionID)
		m.cancelReservation(sessionID, nil)
		return nil, err
	}
	project := projects.Project{ID: header.ProjectID, Root: header.ProjectRoot}
	model := llm.Selection{}
	selected, found := book.LastModelSelection()
	if found {
		model = llm.Selection{Provider: selected.Provider, Model: selected.Model, Thinking: selected.Thinking}
		err = m.llm.Validate(model)
		if err != nil {
			_ = m.books.Release(sessionID)
			m.cancelReservation(sessionID, scope)
			return nil, fmt.Errorf("会话 %s 的模型已不可用：%w", sessionID, err)
		}
	}
	conversation, err := m.prepare(runner, project, preset, model, scope, book, true)
	if err != nil {
		_ = m.books.Release(sessionID)
		m.cancelReservation(sessionID, scope)
		return nil, fmt.Errorf("会话 %s 旧账恢复失败：%w", sessionID, err)
	}
	m.publish(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}
