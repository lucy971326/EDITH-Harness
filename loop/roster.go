package loop

import (
	"fmt"
	"sync"

	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Roster 管长期 Agent 档案和它们正在运行的多段会话。挂在能力名 "agents" 下。
type Roster struct {
	mu       sync.Mutex
	app      *core.App
	store    *session.Store
	profiles ProfileStore
	llmSvc   *llm.Service
	toolsReg *tools.Registry
	agents   map[string]*activeAgent
	sessions map[string]*Conversation
	opening  map[string]bool
}

// activeAgent 是一个正在被至少一段会话使用的 Agent 档案和专属作用域。
type activeAgent struct {
	profile AgentProfile
	scope   *core.App
	uses    int
}

// NewRoster 建一个空名册；档案介质由外面注入，loop 不认识具体文件或数据库。
func NewRoster(app *core.App, store *session.Store, profiles ProfileStore, llmSvc *llm.Service, toolsReg *tools.Registry) *Roster {
	return &Roster{
		app:      app,
		store:    store,
		profiles: profiles,
		llmSvc:   llmSvc,
		toolsReg: toolsReg,
		agents:   make(map[string]*activeAgent),
		sessions: make(map[string]*Conversation),
		opening:  make(map[string]bool),
	}
}

// CreateAgent 保存一份新 Agent 档案；版本从 1 起，工具必须已经由应用安装。
func (r *Roster) CreateAgent(profile AgentProfile) error {
	profile.Revision = 1
	profile.Archived = false
	err := validateProfile(profile)
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
func (r *Roster) UpdateAgent(profile AgentProfile) error {
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
	err = validateProfile(profile)
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
func (r *Roster) ArchiveAgent(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.agents[id]
	if active != nil && active.uses > 0 {
		return fmt.Errorf("agent %s 还有运行中的会话，不能归档", id)
	}
	return r.profiles.Archive(id)
}

// GetAgent 读取一份长期 Agent 档案副本。
func (r *Roster) GetAgent(id string) (AgentProfile, error) {
	profile, err := r.profiles.Get(id)
	if err != nil {
		return AgentProfile{}, err
	}
	return cloneProfile(profile), nil
}

// StartSession 用一个未归档 Agent 开一段新会话。Agent 和会话号都由调用方明确给出。
func (r *Roster) StartSession(agentID string, sessionID string, seed ...session.Event) (*Conversation, error) {
	profile, entry, err := r.reserveSession(agentID, sessionID, false)
	if err != nil {
		return nil, err
	}
	book, err := r.store.Create(sessionID, profile.ID, profile.Revision, seed...)
	if err != nil {
		r.cancelReservation(profile.ID, sessionID)
		return nil, err
	}
	conversation := newConversation(profile.ID, sessionID, entry.scope.ForChild(sessionID), book, r.llmSvc, r.toolsReg, profile)
	conversation.close = r.closeConversation
	if len(seed) > 0 {
		err = recoverFromLedger(conversation, seed)
		if err != nil {
			conversation.scope.Close()
			_ = r.store.Release(sessionID)
			r.cancelReservation(profile.ID, sessionID)
			return nil, fmt.Errorf("会话 %s 旧账恢复失败：%w", sessionID, err)
		}
	}
	r.publishSession(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

// ResumeSession 打开旧会话；所属 Agent 从账本封面读取，后续请求使用它当前档案。
func (r *Roster) ResumeSession(sessionID string) (*Conversation, error) {
	book, err := r.store.Open(sessionID)
	if err != nil {
		return nil, err
	}
	profile, entry, err := r.reserveSession(book.AgentID(), sessionID, true)
	if err != nil {
		_ = r.store.Release(sessionID)
		return nil, err
	}
	conversation := newConversation(profile.ID, sessionID, entry.scope.ForChild(sessionID), book, r.llmSvc, r.toolsReg, profile)
	conversation.close = r.closeConversation
	err = recoverFromLedger(conversation, book.Events())
	if err != nil {
		conversation.scope.Close()
		_ = r.store.Release(sessionID)
		r.cancelReservation(profile.ID, sessionID)
		return nil, fmt.Errorf("会话 %s 旧账恢复失败：%w", sessionID, err)
	}
	r.publishSession(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

// GetSession 按会话号领一个正在运行的会话门面。
func (r *Roster) GetSession(sessionID string) (*Conversation, error) {
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
func (r *Roster) CloseSession(sessionID string) error {
	conversation, err := r.GetSession(sessionID)
	if err != nil {
		return err
	}
	return conversation.Close()
}

// CloseAll 让全部会话收摊；App 收摊时用，单段失败不挡其余。
func (r *Roster) CloseAll() {
	r.mu.Lock()
	conversations := make([]*Conversation, 0, len(r.sessions))
	for _, conversation := range r.sessions {
		conversations = append(conversations, conversation)
	}
	r.mu.Unlock()
	for _, conversation := range conversations {
		_ = conversation.Close()
	}
}

func (r *Roster) acquireAgentLocked(profile AgentProfile) (*activeAgent, bool, error) {
	entry, exists := r.agents[profile.ID]
	if exists {
		return entry, false, nil
	}
	err := r.toolsReg.SetAllowed(profile.ID, profile.Tools)
	if err != nil {
		return nil, false, fmt.Errorf("agent %s 的工具不完整：%w", profile.ID, err)
	}
	entry = &activeAgent{profile: cloneProfile(profile), scope: r.app.ForAgent(profile.ID)}
	r.agents[profile.ID] = entry
	return entry, true, nil
}

// reserveSession 先占住会话号和 Agent 使用数，再放开名册锁做慢操作。
// 这样档案更新不会和会话创建撞车，恢复补账也不会拿着名册锁广播。
func (r *Roster) reserveSession(agentID string, sessionID string, allowArchived bool) (AgentProfile, *activeAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[sessionID]; exists || r.opening[sessionID] {
		return AgentProfile{}, nil, fmt.Errorf("会话 %s 已经开过了", sessionID)
	}
	profile, err := r.profiles.Get(agentID)
	if err != nil {
		return AgentProfile{}, nil, fmt.Errorf("会话 %s 找不到所属 agent %s：%w", sessionID, agentID, err)
	}
	if profile.Archived && !allowArchived {
		return AgentProfile{}, nil, fmt.Errorf("agent %s 已归档，不能开新会话", agentID)
	}
	entry, _, err := r.acquireAgentLocked(profile)
	if err != nil {
		return AgentProfile{}, nil, err
	}
	entry.uses++
	r.opening[sessionID] = true
	return profile, entry, nil
}

func (r *Roster) publishSession(sessionID string, conversation *Conversation) {
	r.mu.Lock()
	delete(r.opening, sessionID)
	r.sessions[sessionID] = conversation
	r.mu.Unlock()
}

func (r *Roster) cancelReservation(agentID string, sessionID string) {
	r.mu.Lock()
	delete(r.opening, sessionID)
	entry := r.agents[agentID]
	if entry != nil {
		entry.uses--
		r.releaseUnusedAgentLocked(agentID)
	}
	r.mu.Unlock()
}

func (r *Roster) releaseUnusedAgentLocked(agentID string) {
	entry, exists := r.agents[agentID]
	if !exists || entry.uses > 0 {
		return
	}
	delete(r.agents, agentID)
	r.toolsReg.DropAgent(agentID)
	entry.scope.Close()
}

func (r *Roster) closeConversation(conversation *Conversation) error {
	conversation.driver.stopAndJoin()
	conversation.scope.Close()
	err := r.store.Release(conversation.sessionID)

	r.mu.Lock()
	if r.sessions[conversation.sessionID] == conversation {
		delete(r.sessions, conversation.sessionID)
		entry := r.agents[conversation.agentID]
		if entry != nil {
			entry.uses--
			r.releaseUnusedAgentLocked(conversation.agentID)
		}
	}
	r.mu.Unlock()
	return err
}
