package loop

import (
	"fmt"

	"harness/agents"
	"harness/llm"
	"harness/tools"
)

// Runner 是 loop 提供的会话搬运工。
type Runner struct {
	llmSvc   *llm.Service
	toolsReg *tools.Registry
}

var _ agents.Runner = (*Runner)(nil)

// NewRunner 建一个会话搬运工。
func NewRunner(llmSvc *llm.Service, toolsReg *tools.Registry) *Runner {
	return &Runner{llmSvc: llmSvc, toolsReg: toolsReg}
}

// Prepare 用 agents 给的材料恢复一段会话，但尚不启动搬运工。
func (r *Runner) Prepare(input agents.RunInput) (agents.PreparedConversation, error) {
	conversation := newConversation(input.SessionID, input.Scope, input.Book, r.llmSvc, r.toolsReg, input.Preset, input.Model, input.ReportError)
	conversation.close = input.Close
	if input.Recover {
		err := recoverFromLedger(conversation, input.Book.Events())
		if err != nil {
			return nil, fmt.Errorf("旧账恢复失败：%w", err)
		}
	}
	return conversation, nil
}
