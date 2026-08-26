package loop

import (
	"context"
	"sync"

	"harness/core"
	"harness/llm"
	"harness/presets"
	"harness/session"
	"harness/tools"
)

// Conversation 是一段会话的门面：UI 和外部服务只跟它说话，门后的搬运工长什么样不用管。
// 两种塞消息：SubmitFollowup 开新轮；Steer 中途捎话（忙时下一步生效，闲时就是新轮）。
type Conversation struct {
	sessionID string
	scope     *core.App // 专属子作用域：控制位、审批人、遮蔽工具都挂这
	book      *session.Session
	llmSvc    *llm.Service
	toolsReg  *tools.Registry
	config    RunConfig
	inbox     *inbox
	driver    *driver

	mu              sync.Mutex
	cond            *sync.Cond // 闲了广播一声，等闲的人全醒
	working         bool       // 搬运工已经接活，可能刚把队列领空、还没正式开轮
	busy            bool       // 正在跑一轮；State 对外报告正式运行
	stepCtx         context.Context
	stepStop        context.CancelFunc
	cancelRequested bool
	close           func() error
	reportError     func(error)
	closeErr        error
	closeOnce       sync.Once
}

func newConversation(sessionID string, scope *core.App, book *session.Session, llmSvc *llm.Service, toolsReg *tools.Registry, preset presets.Revision, model llm.Selection, reportError func(error)) *Conversation {
	conversation := &Conversation{
		sessionID: sessionID,
		scope:     scope,
		book:      book,
		llmSvc:    llmSvc,
		toolsReg:  toolsReg,
		config: RunConfig{
			Provider:     model.Provider,
			Model:        model.Model,
			Thinking:     model.Thinking,
			SystemPrompt: preset.SystemPrompt,
		},
		inbox:       newInbox(),
		reportError: reportError,
	}
	conversation.cond = sync.NewCond(&conversation.mu)
	conversation.driver = newDriver(conversation)
	return conversation
}

// RunConfig 是一次会话当前模型和锁定模式提示词的组合。
type RunConfig struct {
	Provider     string
	Model        string
	Thinking     string
	SystemPrompt string
}

// Start 放搬运工上线。
func (c *Conversation) Start() {
	c.driver.start()
}

// Close 让这段会话下线、账本收口；重复调用安全，第一次的结果会被保留。
func (c *Conversation) Close() error {
	c.closeOnce.Do(func() {
		c.Cancel()
		c.driver.stopAndJoin()
		c.closeErr = c.close()
	})
	return c.closeErr
}

func (c *Conversation) report(cause error) {
	if cause == nil || c.reportError == nil {
		return
	}
	c.reportError(cause)
}

// SessionID 返回这段会话自己的账本号。
func (c *Conversation) SessionID() string {
	return c.sessionID
}

// Book 返回这段会话的账本（UI 读历史、审计用）。
func (c *Conversation) Book() *session.Session {
	return c.book
}
