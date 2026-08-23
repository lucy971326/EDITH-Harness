package loop

import (
	"fmt"
	"sync"

	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Roster 是名册：按 id 开 agent、领 agent。挂在能力名 "agents" 下。
type Roster struct {
	mu       sync.Mutex
	app      *core.App
	store    *session.Store
	llmSvc   *llm.Service
	toolsReg *tools.Registry
	agents   map[string]*Agent
}

// NewRoster 建一个空名册（loop.Plugin 在启动时造好挂进场地）。
func NewRoster(app *core.App, store *session.Store, llmSvc *llm.Service, toolsReg *tools.Registry) *Roster {
	return &Roster{
		app:      app,
		store:    store,
		llmSvc:   llmSvc,
		toolsReg: toolsReg,
		agents:   make(map[string]*Agent),
	}
}

// Create 开一个 agent：开新账本 + 派生专属子作用域 + 放搬运工上线。
// 带 seed = 拿旧账重建（崩溃恢复）：待办找回、悬空的账补齐，然后接着干。
// 同名已存在是组装错误。
func (r *Roster) Create(id string, config AgentConfig, seed ...session.Event) (*Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.agents[id]
	if exists {
		return nil, fmt.Errorf("agent %s 已经开过了", id)
	}

	book, err := r.store.Create(id, seed...)
	if err != nil {
		return nil, err
	}

	agent := newAgent(id, r.app.ForAgent(id), book, r.llmSvc, r.toolsReg, config)
	if len(seed) > 0 {
		err = recoverFromLedger(agent, seed)
		if err != nil {
			return nil, fmt.Errorf("agent %s 旧账恢复失败：%w", id, err)
		}
	}
	r.agents[id] = agent
	agent.Start()
	return agent, nil
}

// Get 按 id 领一个已开的 agent。
func (r *Roster) Get(id string) (*Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent %s 没开过", id)
	}
	return agent, nil
}

// Drop 让一个 agent 收摊下线（名册里除名）。
func (r *Roster) Drop(id string) {
	r.mu.Lock()
	agent, exists := r.agents[id]
	if exists {
		delete(r.agents, id)
	}
	r.mu.Unlock()

	if exists {
		agent.Close()
	}
}

// CloseAll 让全部 agent 收摊（App 收摊时用）。
func (r *Roster) CloseAll() {
	r.mu.Lock()
	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	r.agents = make(map[string]*Agent)
	r.mu.Unlock()

	for _, agent := range agents {
		agent.Close()
	}
}
