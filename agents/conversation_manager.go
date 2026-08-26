package agents

import (
	"fmt"
	"sync"

	"harness/core"
	"harness/environment"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// conversationManager 管理会话从创建到关闭的运行时生命周期。
type conversationManager struct {
	// 保护 Runner 和会话状态。
	mu sync.Mutex

	// 创建会话所需的其他插件能力。
	app         *core.App
	books       *session.Store
	projects    projects.Service
	presets     presets.Service
	environment environment.Provider
	tools       *tools.Registry
	llm         *llm.Service

	// 当前运行时状态。
	runner   Runner
	sessions map[string]Conversation
	opening  map[string]bool
}

var _ Service = (*conversationManager)(nil)

func newConversationManager(
	app *core.App,
	books *session.Store,
	projectService projects.Service,
	presetService presets.Service,
	environmentProvider environment.Provider,
	toolRegistry *tools.Registry,
	llmService *llm.Service,
) *conversationManager {
	return &conversationManager{
		app:         app,
		books:       books,
		projects:    projectService,
		presets:     presetService,
		environment: environmentProvider,
		tools:       toolRegistry,
		llm:         llmService,
		sessions:    make(map[string]Conversation),
		opening:     make(map[string]bool),
	}
}

// OpenSession 取已经运行的会话；若它只是历史账本，则恢复它后返回。
func (m *conversationManager) OpenSession(sessionID string) (Conversation, error) {
	m.mu.Lock()
	conversation, running := m.sessions[sessionID]
	opening := m.opening[sessionID]
	m.mu.Unlock()
	if running {
		return conversation, nil
	}
	if opening {
		return nil, fmt.Errorf("会话 %s 正在准备，稍后再取", sessionID)
	}
	return m.ResumeSession(sessionID)
}

// GetSession 按会话号领取一段正在运行的会话。
func (m *conversationManager) GetSession(sessionID string) (Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conversation, exists := m.sessions[sessionID]
	if !exists {
		if m.opening[sessionID] {
			return nil, fmt.Errorf("会话 %s 正在准备，稍后再取", sessionID)
		}
		return nil, fmt.Errorf("会话 %s 没有运行", sessionID)
	}
	return conversation, nil
}

func (m *conversationManager) reserve(sessionID string) (Runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sessions[sessionID]; exists || m.opening[sessionID] {
		return nil, fmt.Errorf("会话 %s 已经打开", sessionID)
	}
	if m.runner == nil {
		return nil, fmt.Errorf("会话 %s 没有搬运工，请先安装 loop 插件", sessionID)
	}
	m.opening[sessionID] = true
	return m.runner, nil
}

func (m *conversationManager) publish(sessionID string, conversation Conversation) {
	m.mu.Lock()
	delete(m.opening, sessionID)
	m.sessions[sessionID] = conversation
	m.mu.Unlock()
}

func (m *conversationManager) cancelReservation(sessionID string, scope *core.App) {
	if scope != nil {
		scope.Close()
	}
	m.tools.DropScope(sessionID)
	m.mu.Lock()
	delete(m.opening, sessionID)
	m.mu.Unlock()
}
