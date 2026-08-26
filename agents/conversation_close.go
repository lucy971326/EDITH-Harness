package agents

import "harness/core"

// CloseSession 让一段会话收摊；重复关闭由 Conversation 保证安全。
func (m *conversationManager) CloseSession(sessionID string) error {
	conversation, err := m.GetSession(sessionID)
	if err != nil {
		return err
	}
	return conversation.Close()
}

// CloseAll 让全部会话收摊；单段失败不挡其余。
func (m *conversationManager) CloseAll() {
	m.mu.Lock()
	conversations := make([]Conversation, 0, len(m.sessions))
	for _, conversation := range m.sessions {
		conversations = append(conversations, conversation)
	}
	m.mu.Unlock()
	for _, conversation := range conversations {
		_ = conversation.Close()
	}
}

func (m *conversationManager) finish(sessionID string, scope *core.App) error {
	scope.Close()
	m.tools.DropScope(sessionID)
	err := m.books.Release(sessionID)
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	return err
}
