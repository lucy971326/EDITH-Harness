package loop

import (
	"context"
	"sync"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Conversation 是一段会话的门面：UI 和外部服务只跟它说话，门后的搬运工长什么样不用管。
// 三种塞消息：SubmitFollowup 开新轮；Steer 中途捎话（忙时下一步生效，闲时就是新轮）；
// InjectMemo 塞小抄（不吵醒，下次问模型时拼进上下文）。
type Conversation struct {
	agentID   string
	sessionID string
	scope     *core.App // 专属子作用域：控制位、审批人、遮蔽工具都挂这
	book      *session.Session
	llmSvc    *llm.Service
	toolsReg  *tools.Registry
	config    AgentConfig
	inbox     *inbox
	driver    *driver

	mu        sync.Mutex
	cond      *sync.Cond // 闲了广播一声，等闲的人全醒
	working   bool       // 搬运工已经接活，可能刚把队列领空、还没正式开轮
	busy      bool       // 正在跑一轮；State 对外只报这个
	stepCtx   context.Context
	stepStop  context.CancelFunc
	close     func() error
	closeErr  error
	closeOnce sync.Once
}

func newConversation(agentID string, sessionID string, scope *core.App, book *session.Session, llmSvc *llm.Service, toolsReg *tools.Registry, profile agents.AgentProfile) *Conversation {
	conversation := &Conversation{
		agentID:   agentID,
		sessionID: sessionID,
		scope:     scope,
		book:      book,
		llmSvc:    llmSvc,
		toolsReg:  toolsReg,
		config:    AgentConfig{Model: profile.Model, SystemPrompt: profile.SystemPrompt},
		inbox:     newInbox(),
	}
	conversation.cond = sync.NewCond(&conversation.mu)
	conversation.driver = newDriver(conversation)
	return conversation
}

// AgentConfig 是一次会话运行时从 Agent 档案取出的模型配置。
type AgentConfig struct {
	Model        string
	SystemPrompt string
}

// Start 放搬运工上线。
func (a *Conversation) Start() {
	a.driver.start()
}

// Close 让这段会话下线、账本收口；重复调用安全，第一次的结果会被保留。
func (a *Conversation) Close() error {
	a.closeOnce.Do(func() {
		a.closeErr = a.close()
	})
	return a.closeErr
}

// AgentID 返回这段会话属于哪个长期 Agent。
func (a *Conversation) AgentID() string {
	return a.agentID
}

// SessionID 返回这段会话自己的账本号。
func (a *Conversation) SessionID() string {
	return a.sessionID
}

// State 返回 "idle"（闲）或 "busy"（忙）。
func (a *Conversation) State() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.busy {
		return "busy"
	}
	return "idle"
}

// WaitIdle 一直等到这段会话把手上的活干完：不在干活、且待办队列空。
// 只看"忙"不够——消息刚投进来还没开跑的那一瞬也是"不忙"，等了等于没等。
func (a *Conversation) WaitIdle() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for a.working || a.busy || a.inbox.pending() {
		a.cond.Wait()
	}
}

// Cancel 取消当前正在跑的步（模型请求、工具执行都会收到取消信号）。
// 已吐的字和工具的善后按账本规矩走，绝不装作没发生。
func (a *Conversation) Cancel() {
	a.mu.Lock()
	stop := a.stepStop
	a.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// SubmitFollowup 投一条开新轮的消息：忙时排队，本轮正常结束后才开新轮。
func (a *Conversation) SubmitFollowup(text string) error {
	return a.inbox.deliver(a.book, TargetNextTurn, text, true)
}

// Steer 中途捎话：忙时进当前轮的下一步；闲时没人可打扰，就当新轮的开头。
func (a *Conversation) Steer(text string) error {
	return a.inbox.deliver(a.book, TargetNextStep, text, true)
}

// InjectMemo 塞一张小抄：不响铃不占待办，下次问模型时拼进上下文，不变成用户的话。
func (a *Conversation) InjectMemo(text string) error {
	return a.inbox.deliver(a.book, TargetMemo, text, false)
}

// Book 返回这段会话的账本（UI 读历史、审计用）。
func (a *Conversation) Book() *session.Session {
	return a.book
}

// claimAsUserMessage 领出一份投递并落成用户的话——领出才进模型，两条账在这合上。
func (a *Conversation) claimAsUserMessage(item delivery) error {
	_, err := a.book.RecordClaim(item.ID)
	if err != nil {
		return err
	}
	_, err = a.book.RecordUserMessage(item.Text)
	return err
}

// stepContext 拿当前步的取消口；每步一个，Cancel 掐的就是它。
func (a *Conversation) stepContext() context.Context {
	ctx, stop := context.WithCancel(context.Background())
	a.mu.Lock()
	a.stepCtx = ctx
	a.stepStop = stop
	a.mu.Unlock()
	return ctx
}

// markWorking 先占住待办，防止队列刚领空时 WaitIdle 误判已经干完。
func (a *Conversation) markWorking() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.working = true
}

// markBusy 报告一轮已经正式开跑。
func (a *Conversation) markBusy() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.busy = true
}

// markTurnDone 报告这一轮结束；搬运工可能还要接着办下一轮。
func (a *Conversation) markTurnDone() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.busy = false
}

// markIdle 报闲并广播：所有等闲的人一起醒。
func (a *Conversation) markIdle() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.working = false
	a.busy = false
	a.cond.Broadcast()
}
