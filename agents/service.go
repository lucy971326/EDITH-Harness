package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"harness/core"
	"harness/environment"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// Conversation 是一段运行中会话对外开放的操作。
type Conversation interface {
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

// ConversationError 是会话运行中的实时故障通知；它不写进账本。
type ConversationError struct {
	SessionID string // 哪段会话
	Message   string // 给界面显示的失败原因
}

// EventConversationError 是会话运行失败时的实时事件名。
const EventConversationError = "agents/conversation-error"

// StartInput 是新会话第一次启动时锁定的项目、模式和标题。
type StartInput struct {
	ProjectID string
	PresetID  string
	Title     string
}

// PreparedConversation 是已恢复、尚未开始搬运消息的会话。
type PreparedConversation interface {
	Conversation
	Start()
}

// RunInput 是 agents 交给 Runner 的一段会话材料。
type RunInput struct {
	SessionID   string
	Project     projects.Project
	Preset      presets.Revision
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

// Service 是其他插件管理运行中会话的公共入口。
type Service interface {
	StartSession(input StartInput, seed ...session.Event) (Conversation, error)
	ResumeSession(sessionID string) (Conversation, error)
	GetSession(sessionID string) (Conversation, error)
	CloseSession(sessionID string) error
	RegisterRunner(runner Runner) (func(), error)
}

// registry 只管会话运行时；项目和模式分别由自己的插件保存。
type registry struct {
	mu          sync.Mutex
	app         *core.App
	books       *session.Store
	projects    projects.Service
	presets     presets.Service
	environment environment.Provider
	tools       *tools.Registry
	runner      Runner
	sessions    map[string]Conversation
	opening     map[string]bool
}

func newRegistry(app *core.App, books *session.Store, projectService projects.Service, presetService presets.Service, provider environment.Provider, toolsReg *tools.Registry) *registry {
	return &registry{
		app:         app,
		books:       books,
		projects:    projectService,
		presets:     presetService,
		environment: provider,
		tools:       toolsReg,
		sessions:    make(map[string]Conversation),
		opening:     make(map[string]bool),
	}
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

// StartSession 用项目和模式当前版本开一段新会话，返回自动生成身份的运行门面。
func (r *registry) StartSession(input StartInput, seed ...session.Event) (Conversation, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, fmt.Errorf("会话标题不能为空")
	}
	project, err := r.projects.Get(input.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("打开项目 %s 失败：%w", input.ProjectID, err)
	}
	if project.Archived {
		return nil, fmt.Errorf("项目 %s 已归档，不能开新会话", project.ID)
	}
	preset, err := r.presets.Get(input.PresetID)
	if err != nil {
		return nil, fmt.Errorf("读取 Agent 模式 %s 失败：%w", input.PresetID, err)
	}
	if preset.Archived {
		return nil, fmt.Errorf("Agent 模式 %s 已归档，不能开新会话", preset.ID)
	}
	err = presets.ValidateRunnable(preset)
	if err != nil {
		return nil, err
	}
	err = r.tools.CheckAllowed(preset.Tools)
	if err != nil {
		return nil, fmt.Errorf("Agent 模式 %s 的工具不完整：%w", preset.ID, err)
	}
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	runner, err := r.reserve(sessionID)
	if err != nil {
		return nil, err
	}
	header := session.Header{
		ID:             sessionID,
		Title:          input.Title,
		CreatedAt:      time.Now().UTC(),
		ProjectID:      project.ID,
		ProjectRoot:    project.Root,
		PresetID:       preset.ID,
		PresetRevision: preset.Revision,
	}
	conversation, err := r.openNew(runner, project, preset, header, seed)
	if err != nil {
		r.cancelReservation(sessionID, nil)
		return nil, err
	}
	r.publish(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

func (r *registry) openNew(runner Runner, project projects.Project, preset presets.Revision, header session.Header, seed []session.Event) (PreparedConversation, error) {
	scope, err := r.mountScope(header.ID, header.ProjectRoot, preset.Tools)
	if err != nil {
		return nil, err
	}
	book, err := r.books.Create(header, seed...)
	if err != nil {
		r.cancelReservation(header.ID, scope)
		return nil, err
	}
	conversation, err := r.prepare(runner, project, preset, scope, book, len(seed) > 0)
	if err != nil {
		_ = r.books.Release(header.ID)
		r.cancelReservation(header.ID, scope)
		return nil, fmt.Errorf("会话 %s 准备失败：%w", header.ID, err)
	}
	return conversation, nil
}

// ResumeSession 按账本封面恢复旧会话，继续使用当时锁定的模式版本和项目根目录。
func (r *registry) ResumeSession(sessionID string) (Conversation, error) {
	book, err := r.books.Open(sessionID)
	if err != nil {
		return nil, err
	}
	header := book.Header()
	preset, err := r.presets.GetRevision(header.PresetID, header.PresetRevision)
	if err != nil {
		_ = r.books.Release(sessionID)
		return nil, fmt.Errorf("会话 %s 的 Agent 模式版本不存在：%w", sessionID, err)
	}
	err = presets.ValidateRunnable(preset)
	if err != nil {
		_ = r.books.Release(sessionID)
		return nil, err
	}
	err = r.tools.CheckAllowed(preset.Tools)
	if err != nil {
		_ = r.books.Release(sessionID)
		return nil, fmt.Errorf("会话 %s 的历史工具不完整：%w", sessionID, err)
	}
	runner, err := r.reserve(sessionID)
	if err != nil {
		_ = r.books.Release(sessionID)
		return nil, err
	}
	scope, err := r.mountScope(sessionID, header.ProjectRoot, preset.Tools)
	if err != nil {
		_ = r.books.Release(sessionID)
		r.cancelReservation(sessionID, nil)
		return nil, err
	}
	project := projects.Project{ID: header.ProjectID, Root: header.ProjectRoot}
	conversation, err := r.prepare(runner, project, preset, scope, book, true)
	if err != nil {
		_ = r.books.Release(sessionID)
		r.cancelReservation(sessionID, scope)
		return nil, fmt.Errorf("会话 %s 旧账恢复失败：%w", sessionID, err)
	}
	r.publish(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

func (r *registry) mountScope(sessionID string, root string, allowed []string) (*core.App, error) {
	scope := r.app.ForChild(sessionID)
	err := r.environment.Mount(scope, root)
	if err != nil {
		scope.Close()
		return nil, fmt.Errorf("会话 %s 挂载执行环境失败：%w", sessionID, err)
	}
	err = r.tools.SetAllowedForScope(sessionID, allowed)
	if err != nil {
		scope.Close()
		return nil, err
	}
	return scope, nil
}

func (r *registry) prepare(runner Runner, project projects.Project, preset presets.Revision, scope *core.App, book *session.Session, recover bool) (PreparedConversation, error) {
	sessionID := book.ID()
	input := RunInput{
		SessionID: sessionID,
		Project:   project,
		Preset:    preset,
		Scope:     scope,
		Book:      book,
		Recover:   recover,
		Close: func() error {
			return r.finish(sessionID, scope)
		},
		ReportError: func(cause error) {
			if cause == nil {
				return
			}
			r.app.Broadcast(EventConversationError, ConversationError{
				SessionID: sessionID,
				Message:   cause.Error(),
			})
		},
	}
	return runner.Prepare(input)
}

// GetSession 按会话号领取一段正在运行的会话。
func (r *registry) GetSession(sessionID string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conversation, exists := r.sessions[sessionID]
	if !exists {
		if r.opening[sessionID] {
			return nil, fmt.Errorf("会话 %s 正在准备，稍后再取", sessionID)
		}
		return nil, fmt.Errorf("会话 %s 没有运行", sessionID)
	}
	return conversation, nil
}

// CloseSession 让一段会话收摊；重复关闭由 Conversation 保证安全。
func (r *registry) CloseSession(sessionID string) error {
	conversation, err := r.GetSession(sessionID)
	if err != nil {
		return err
	}
	return conversation.Close()
}

// CloseAll 让全部会话收摊；单段失败不挡其余。
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

func (r *registry) reserve(sessionID string) (Runner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[sessionID]; exists || r.opening[sessionID] {
		return nil, fmt.Errorf("会话 %s 已经打开", sessionID)
	}
	if r.runner == nil {
		return nil, fmt.Errorf("会话 %s 没有搬运工，请先安装 loop 插件", sessionID)
	}
	r.opening[sessionID] = true
	return r.runner, nil
}

func (r *registry) publish(sessionID string, conversation Conversation) {
	r.mu.Lock()
	delete(r.opening, sessionID)
	r.sessions[sessionID] = conversation
	r.mu.Unlock()
}

func (r *registry) cancelReservation(sessionID string, scope *core.App) {
	if scope != nil {
		scope.Close()
	}
	r.tools.DropScope(sessionID)
	r.mu.Lock()
	delete(r.opening, sessionID)
	r.mu.Unlock()
}

func (r *registry) finish(sessionID string, scope *core.App) error {
	scope.Close()
	r.tools.DropScope(sessionID)
	err := r.books.Release(sessionID)
	r.mu.Lock()
	delete(r.sessions, sessionID)
	r.mu.Unlock()
	return err
}

func generateSessionID() (string, error) {
	data := make([]byte, 16)
	_, err := rand.Read(data)
	if err != nil {
		return "", fmt.Errorf("生成会话 id 失败：%w", err)
	}
	return hex.EncodeToString(data), nil
}
