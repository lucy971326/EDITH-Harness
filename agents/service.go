package agents

import (
	"fmt"
	"sync"

	"harness/core"
	"harness/session"
	"harness/tools"
)

// Conversation 是一段运行中会话对外开放的操作。
type Conversation interface {
	AgentID() string
	SessionID() string
	State() string
	WaitIdle()
	Cancel()
	SubmitFollowup(text string) error
	Steer(text string) error
	InjectMemo(text string) error
	Book() *session.Session
	Close() error
}

// PreparedConversation 是已恢复、尚未开始搬运消息的会话。
type PreparedConversation interface {
	Conversation
	Start()
}

// RunInput 是 agents 交给 Runner 的一段会话材料。
type RunInput struct {
	AgentID   string
	SessionID string
	Profile   AgentProfile
	Scope     *core.App
	Book      *session.Session
	Recover   bool
	Close     func() error
}

// Runner 负责把一段准备好的会话跑起来；loop.Plugin 提供默认实现。
type Runner interface {
	Prepare(input RunInput) (PreparedConversation, error)
}

// Service 是其他插件管理 Agent 的公共入口。
type Service interface {
	CreateAgent(profile AgentProfile) error
	UpdateAgent(profile AgentProfile) error
	ArchiveAgent(id string) error
	GetAgent(id string) (AgentProfile, error)
	StartSession(agentID string, sessionID string, seed ...session.Event) (Conversation, error)
	ResumeSession(sessionID string) (Conversation, error)
	GetSession(sessionID string) (Conversation, error)
	CloseSession(sessionID string) error
	RegisterRunner(runner Runner) (func(), error)
}

// registry 管长期 Agent 档案和它们正在运行的多段会话。
type registry struct {
	mu       sync.Mutex
	app      *core.App
	store    *session.Store
	profiles ProfileStore
	toolsReg *tools.Registry
	runner   Runner
	agents   map[string]*activeAgent
	sessions map[string]Conversation
	opening  map[string]bool
}

// activeAgent 是一个正在被至少一段会话使用的 Agent 档案和专属作用域。
type activeAgent struct {
	profile AgentProfile
	scope   *core.App
	uses    int
}

func newRegistry(app *core.App, store *session.Store, profiles ProfileStore, toolsReg *tools.Registry) *registry {
	return &registry{
		app:      app,
		store:    store,
		profiles: profiles,
		toolsReg: toolsReg,
		agents:   make(map[string]*activeAgent),
		sessions: make(map[string]Conversation),
		opening:  make(map[string]bool),
	}
}

// CreateAgent 保存一份新 Agent 档案；版本从 1 起，工具必须已经由应用安装。
func (r *registry) CreateAgent(profile AgentProfile) error {
	profile.Revision = 1
	profile.Archived = false
	err := ValidateRunnableProfile(profile)
	if err != nil {
		return err
	}
	err = r.toolsReg.CheckAllowed(profile.Tools)
	if err != nil {
		return fmt.Errorf("agent %s 的工具不完整：%w", profile.ID, err)
	}
	return r.profiles.Create(cloneProfile(profile))
}

// UpdateAgent 保存 Agent 的新版档案；有运行中会话时拒绝，避免半轮换人设。
func (r *registry) UpdateAgent(profile AgentProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.agents[profile.ID]
	if active != nil && active.uses > 0 {
		return fmt.Errorf("agent %s 还有运行中的会话，不能更新", profile.ID)
	}

	old, err := r.profiles.Get(profile.ID)
	if err != nil {
		return err
	}
	profile.Revision = old.Revision + 1
	profile.Archived = old.Archived
	err = ValidateRunnableProfile(profile)
	if err != nil {
		return err
	}
	err = r.toolsReg.CheckAllowed(profile.Tools)
	if err != nil {
		return fmt.Errorf("agent %s 的工具不完整：%w", profile.ID, err)
	}
	return r.profiles.Update(cloneProfile(profile))
}

// ArchiveAgent 归档一个不在运行的 Agent；不能再开新会话，历史会话仍可恢复。
func (r *registry) ArchiveAgent(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.agents[id]
	if active != nil && active.uses > 0 {
		return fmt.Errorf("agent %s 还有运行中的会话，不能归档", id)
	}
	return r.profiles.Archive(id)
}

// GetAgent 读取一份长期 Agent 档案副本。
func (r *registry) GetAgent(id string) (AgentProfile, error) {
	profile, err := r.profiles.Get(id)
	if err != nil {
		return AgentProfile{}, err
	}
	return cloneProfile(profile), nil
}

// RegisterRunner 登记唯一会话搬运工，返回取消登记它的函数。
func (r *registry) RegisterRunner(runner Runner) (func(), error) {
	if runner == nil {
		return nil, fmt.Errorf("Agent Runner 不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runner != nil {
		return nil, fmt.Errorf("Agent Runner 已登记，不能重复登记")
	}
	r.runner = runner
	return func() {
		r.mu.Lock()
		if r.runner == runner {
			r.runner = nil
		}
		r.mu.Unlock()
	}, nil
}

// StartSession 用一个未归档 Agent 开一段新会话。Agent 和会话号都由调用方明确给出。
func (r *registry) StartSession(agentID string, sessionID string, seed ...session.Event) (Conversation, error) {
	profile, entry, runner, err := r.reserveSession(agentID, sessionID, false)
	if err != nil {
		return nil, err
	}
	book, err := r.store.Create(sessionID, profile.ID, profile.Revision, seed...)
	if err != nil {
		r.cancelReservation(profile.ID, sessionID)
		return nil, err
	}
	conversation, err := r.prepareConversation(runner, profile, entry, sessionID, book, len(seed) > 0)
	if err != nil {
		_ = r.store.Release(sessionID)
		r.cancelReservation(profile.ID, sessionID)
		return nil, fmt.Errorf("会话 %s 准备失败：%w", sessionID, err)
	}
	r.publishSession(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

// ResumeSession 打开旧会话；所属 Agent 从账本封面读取，后续请求使用它当前档案。
func (r *registry) ResumeSession(sessionID string) (Conversation, error) {
	book, err := r.store.Open(sessionID)
	if err != nil {
		return nil, err
	}
	profile, entry, runner, err := r.reserveSession(book.AgentID(), sessionID, true)
	if err != nil {
		_ = r.store.Release(sessionID)
		return nil, err
	}
	conversation, err := r.prepareConversation(runner, profile, entry, sessionID, book, true)
	if err != nil {
		_ = r.store.Release(sessionID)
		r.cancelReservation(profile.ID, sessionID)
		return nil, fmt.Errorf("会话 %s 旧账恢复失败：%w", sessionID, err)
	}
	r.publishSession(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

func (r *registry) prepareConversation(runner Runner, profile AgentProfile, entry *activeAgent, sessionID string, book *session.Session, recover bool) (PreparedConversation, error) {
	scope := entry.scope.ForChild(sessionID)
	input := RunInput{
		AgentID:   profile.ID,
		SessionID: sessionID,
		Profile:   profile,
		Scope:     scope,
		Book:      book,
		Recover:   recover,
		Close: func() error {
			return r.finishSession(profile.ID, sessionID, scope)
		},
	}
	conversation, err := runner.Prepare(input)
	if err != nil {
		scope.Close()
		return nil, err
	}
	return conversation, nil
}

// GetSession 按会话号领一个正在运行的会话门面。
func (r *registry) GetSession(sessionID string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conversation, exists := r.sessions[sessionID]
	if !exists {
		if r.opening[sessionID] {
			return nil, fmt.Errorf("会话 %s 正在准备，稍后再取", sessionID)
		}
		return nil, fmt.Errorf("会话 %s 没开过", sessionID)
	}
	return conversation, nil
}

// CloseSession 让一段会话收摊；重复关闭安全。
func (r *registry) CloseSession(sessionID string) error {
	conversation, err := r.GetSession(sessionID)
	if err != nil {
		return err
	}
	return conversation.Close()
}

// CloseAll 让全部会话收摊；App 收摊时用，单段失败不挡其余。
func (r *registry) CloseAll() {
	r.mu.Lock()
	conversations := make([]Conversation, 0, len(r.sessions))
	for _, conversation := range r.sessions {
		conversations = append(conversations, conversation)
	}
	r.mu.Unlock()
	for _, conversation := range conversations {
		_ = conversation.Close()
	}
}

func (r *registry) acquireAgentLocked(profile AgentProfile) (*activeAgent, error) {
	entry, exists := r.agents[profile.ID]
	if exists {
		return entry, nil
	}
	err := r.toolsReg.SetAllowed(profile.ID, profile.Tools)
	if err != nil {
		return nil, fmt.Errorf("agent %s 的工具不完整：%w", profile.ID, err)
	}
	entry = &activeAgent{profile: cloneProfile(profile), scope: r.app.ForAgent(profile.ID)}
	r.agents[profile.ID] = entry
	return entry, nil
}

func (r *registry) reserveSession(agentID string, sessionID string, allowArchived bool) (AgentProfile, *activeAgent, Runner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[sessionID]; exists || r.opening[sessionID] {
		return AgentProfile{}, nil, nil, fmt.Errorf("会话 %s 已经开过了", sessionID)
	}
	if r.runner == nil {
		return AgentProfile{}, nil, nil, fmt.Errorf("会话 %s 没有搬运工，请先安装 loop 插件", sessionID)
	}
	profile, err := r.profiles.Get(agentID)
	if err != nil {
		return AgentProfile{}, nil, nil, fmt.Errorf("会话 %s 找不到所属 agent %s：%w", sessionID, agentID, err)
	}
	if profile.Archived && !allowArchived {
		return AgentProfile{}, nil, nil, fmt.Errorf("agent %s 已归档，不能开新会话", agentID)
	}
	err = ValidateRunnableProfile(profile)
	if err != nil {
		return AgentProfile{}, nil, nil, err
	}
	entry, err := r.acquireAgentLocked(profile)
	if err != nil {
		return AgentProfile{}, nil, nil, err
	}
	entry.uses++
	r.opening[sessionID] = true
	return profile, entry, r.runner, nil
}

func (r *registry) publishSession(sessionID string, conversation Conversation) {
	r.mu.Lock()
	delete(r.opening, sessionID)
	r.sessions[sessionID] = conversation
	r.mu.Unlock()
}

func (r *registry) cancelReservation(agentID string, sessionID string) {
	r.mu.Lock()
	delete(r.opening, sessionID)
	entry := r.agents[agentID]
	if entry != nil {
		entry.uses--
		r.releaseUnusedAgentLocked(agentID)
	}
	r.mu.Unlock()
}

func (r *registry) finishSession(agentID string, sessionID string, scope *core.App) error {
	scope.Close()
	err := r.store.Release(sessionID)

	r.mu.Lock()
	if _, exists := r.sessions[sessionID]; exists {
		delete(r.sessions, sessionID)
		entry := r.agents[agentID]
		if entry != nil {
			entry.uses--
			r.releaseUnusedAgentLocked(agentID)
		}
	}
	r.mu.Unlock()
	return err
}

func (r *registry) releaseUnusedAgentLocked(agentID string) {
	entry, exists := r.agents[agentID]
	if !exists || entry.uses > 0 {
		return
	}
	delete(r.agents, agentID)
	r.toolsReg.DropAgent(agentID)
	entry.scope.Close()
}
