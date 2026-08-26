package agents

import (
	"fmt"

	"harness/core"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
)

// RunInput 是 agents 交给 Runner 的一段会话材料。
type RunInput struct {
	SessionID   string
	Project     projects.Project
	Preset      presets.Revision
	Model       llm.Selection
	Scope       *core.App
	Book        *session.Session
	Recover     bool
	Close       func() error
	ReportError func(error)
}

// Runner 负责把一段准备好的会话跑起来；loop.Plugin 提供默认实现。
type Runner interface {
	Prepare(input RunInput) (PreparedConversation, error)
}

// RegisterRunner 登记唯一会话搬运工，返回取消登记它的函数。
func (m *conversationManager) RegisterRunner(runner Runner) (func(), error) {
	if runner == nil {
		return nil, fmt.Errorf("Agent Runner 不能为空")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runner != nil {
		return nil, fmt.Errorf("Agent Runner 已登记，不能重复登记")
	}
	m.runner = runner
	return func() {
		m.mu.Lock()
		if m.runner == runner {
			m.runner = nil
		}
		m.mu.Unlock()
	}, nil
}
