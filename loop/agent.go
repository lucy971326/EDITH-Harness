package loop

import (
	"context"
	"sync"

	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// AgentConfig 是开一个 agent 的配置。
type AgentConfig struct {
	Model        string // 用插座排的默认型号（将来可指名服务商）
	SystemPrompt string // 系统提示词，可空
}

// Agent 是门面：UI 和外部服务只跟它说话，门后的搬运工长什么样不用管。
// 三种塞消息：SubmitFollowup 开新轮；Steer 中途捎话（忙时下一步生效，闲时就是新轮）；
// InjectMemo 塞小抄（不吵醒，下次问模型时拼进上下文）。
type Agent struct {
	id       string
	scope    *core.App // 专属子作用域：控制位、审批人、遮蔽工具都挂这
	book     *session.Session
	llmSvc   *llm.Service
	toolsReg *tools.Registry
	config   AgentConfig
	inbox    *inbox
	driver   *driver

	mu       sync.Mutex
	cond     *sync.Cond // 闲了广播一声，等闲的人全醒
	working  bool       // 搬运工已经接活，可能刚把队列领空、还没正式开轮
	busy     bool       // 正在跑一轮；State 对外只报这个
	stepCtx  context.Context
	stepStop context.CancelFunc
}

func newAgent(id string, scope *core.App, book *session.Session, llmSvc *llm.Service, toolsReg *tools.Registry, config AgentConfig) *Agent {
	agent := &Agent{
		id:       id,
		scope:    scope,
		book:     book,
		llmSvc:   llmSvc,
		toolsReg: toolsReg,
		config:   config,
		inbox:    newInbox(),
	}
	agent.cond = sync.NewCond(&agent.mu)
	agent.driver = newDriver(agent)
	return agent
}

// Start 放搬运工上线。
func (a *Agent) Start() {
	a.driver.start()
}

// Close 让搬运工下线、作用域收摊。
func (a *Agent) Close() {
	a.driver.stopAndJoin()
	a.toolsReg.DropAgent(a.id)
	a.scope.Close()
}

// ID 返回 agent 名。
func (a *Agent) ID() string {
	return a.id
}

// State 返回 "idle"（闲）或 "busy"（忙）。
func (a *Agent) State() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.busy {
		return "busy"
	}
	return "idle"
}

// WaitIdle 一直等到 agent 把手上的活干完：不在干活、且待办队列空。
// 只看"忙"不够——消息刚投进来还没开跑的那一瞬也是"不忙"，等了等于没等。
func (a *Agent) WaitIdle() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for a.working || a.busy || a.inbox.pending() {
		a.cond.Wait()
	}
}

// Cancel 取消当前正在跑的步（模型请求、工具执行都会收到取消信号）。
// 已吐的字和工具的善后按账本规矩走，绝不装作没发生。
func (a *Agent) Cancel() {
	a.mu.Lock()
	stop := a.stepStop
	a.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// SubmitFollowup 投一条开新轮的消息：忙时排队，本轮正常结束后才开新轮。
func (a *Agent) SubmitFollowup(text string) error {
	return a.inbox.deliver(a.book, TargetNextTurn, text, true)
}

// Steer 中途捎话：忙时进当前轮的下一步；闲时没人可打扰，就当新轮的开头。
func (a *Agent) Steer(text string) error {
	return a.inbox.deliver(a.book, TargetNextStep, text, true)
}

// InjectMemo 塞一张小抄：不响铃不占待办，下次问模型时拼进上下文，不变成用户的话。
func (a *Agent) InjectMemo(text string) error {
	return a.inbox.deliver(a.book, TargetMemo, text, false)
}

// Book 返回这个 agent 的账本（UI 读历史、审计用）。
func (a *Agent) Book() *session.Session {
	return a.book
}

// claimAsUserMessage 领出一份投递并落成用户的话——领出才进模型，两条账在这合上。
func (a *Agent) claimAsUserMessage(item delivery) error {
	_, err := a.book.RecordClaim(item.ID)
	if err != nil {
		return err
	}
	_, err = a.book.RecordUserMessage(item.Text)
	return err
}

// stepContext 拿当前步的取消口；每步一个，Cancel 掐的就是它。
func (a *Agent) stepContext() context.Context {
	ctx, stop := context.WithCancel(context.Background())
	a.mu.Lock()
	a.stepCtx = ctx
	a.stepStop = stop
	a.mu.Unlock()
	return ctx
}

// markWorking 先占住待办，防止队列刚领空时 WaitIdle 误判已经干完。
func (a *Agent) markWorking() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.working = true
}

// markBusy 报告一轮已经正式开跑。
func (a *Agent) markBusy() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.busy = true
}

// markTurnDone 报告这一轮结束；搬运工可能还要接着办下一轮。
func (a *Agent) markTurnDone() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.busy = false
}

// markIdle 报闲并广播：所有等闲的人一起醒。
func (a *Agent) markIdle() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.working = false
	a.busy = false
	a.cond.Broadcast()
}
