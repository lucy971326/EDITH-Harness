package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/localenv"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// Roster 是测试用门面，让旧测试通过公开 agents.Service 进入系统。
type Roster = testRoster
type ProfileStore = presets.Store
type AgentProfile struct {
	ID           string
	Provider     string
	Model        string
	Thinking     string
	SystemPrompt string
	Tools        []string
	Archived     bool
}

type testRoster struct {
	agents.Service
	mu        sync.Mutex
	projects  projects.Service
	presets   presets.Service
	books     *session.Store
	projectID string
	toolsReg  *tools.Registry
	profiles  map[string]llm.Selection
}

func cloneProfile(profile presets.Revision) presets.Revision {
	copied := profile
	copied.Tools = append([]string(nil), profile.Tools...)
	return copied
}

func (r *testRoster) CreateAgent(profile AgentProfile) error {
	err := r.presets.Create(presets.Preset{ID: profile.ID, SystemPrompt: profile.SystemPrompt, Tools: profile.Tools, Archived: profile.Archived})
	if err == nil {
		r.mu.Lock()
		r.profiles[profile.ID] = llm.Selection{Provider: profile.Provider, Model: profile.Model, Thinking: profile.Thinking}
		r.mu.Unlock()
	}
	return err
}

func (r *testRoster) UpdateAgent(profile AgentProfile) error {
	err := r.presets.Update(presets.Preset{ID: profile.ID, SystemPrompt: profile.SystemPrompt, Tools: profile.Tools, Archived: profile.Archived})
	if err == nil {
		r.mu.Lock()
		r.profiles[profile.ID] = llm.Selection{Provider: profile.Provider, Model: profile.Model, Thinking: profile.Thinking}
		r.mu.Unlock()
	}
	return err
}

func (r *testRoster) ArchiveAgent(id string) error {
	return r.presets.Archive(id)
}

func (r *testRoster) StartSession(presetID string, title string, seed ...session.Event) (agents.Conversation, error) {
	r.mu.Lock()
	selection := r.profiles[presetID]
	r.mu.Unlock()
	if selection.Provider == "" {
		selection = llm.Selection{Provider: "剧本", Model: "test-model", Thinking: "off"}
	}
	return r.Service.StartSession(agents.StartInput{ProjectID: r.projectID, PresetID: presetID, Title: title, Model: selection}, seed...)
}

func (r *testRoster) ResumeSession(idOrTitle string) (agents.Conversation, error) {
	id := r.resolveSessionID(idOrTitle)
	return r.Service.ResumeSession(id)
}

func (r *testRoster) GetSession(idOrTitle string) (agents.Conversation, error) {
	id := r.resolveSessionID(idOrTitle)
	return r.Service.GetSession(id)
}

func (r *testRoster) resolveSessionID(idOrTitle string) string {
	headers, err := r.books.ListHeaders()
	if err != nil {
		return idOrTitle
	}
	for _, header := range headers {
		if header.ID == idOrTitle || header.Title == idOrTitle {
			return header.ID
		}
	}
	return idOrTitle
}

// silentBroadcaster 给账本一个不出声的广播口。
type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

// memoryJournalPlugin 给 loop 测试装内存账本，避免测试碰本地文件。
type memoryJournalPlugin struct {
	journal session.Journal
}

// memoryProfilePlugin 给 loop 测试装内存模式库。
type memoryProfilePlugin struct {
	store ProfileStore
}

func (memoryProfilePlugin) Name() string { return "test-memory-profiles" }

func (p memoryProfilePlugin) Start(app *core.App) error {
	app.RegisterService("preset-store", p.store)
	return nil
}

type memoryProjectPlugin struct {
	store *memoryProjectStore
}

func (memoryProjectPlugin) Name() string { return "test-memory-projects" }

func (p memoryProjectPlugin) Start(app *core.App) error {
	app.RegisterService("project-store", projects.Store(p.store))
	return nil
}

type memoryProjectStore struct {
	mu       sync.Mutex
	projects map[string]projects.Project
}

func newMemoryProjectStore() *memoryProjectStore {
	return &memoryProjectStore{projects: make(map[string]projects.Project)}
}

func (s *memoryProjectStore) Create(project projects.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[project.ID]; exists {
		return fmt.Errorf("项目 %s 已存在", project.ID)
	}
	s.projects[project.ID] = project
	return nil
}

func (s *memoryProjectStore) Get(id string) (projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, exists := s.projects[id]
	if !exists {
		return projects.Project{}, fmt.Errorf("项目 %s 不存在", id)
	}
	return project, nil
}

func (s *memoryProjectStore) List() ([]projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := make([]projects.Project, 0, len(s.projects))
	for _, project := range s.projects {
		listed = append(listed, project)
	}
	return listed, nil
}

func (s *memoryProjectStore) Update(project projects.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[project.ID] = project
	return nil
}

// blockingProfileStore 让更新停在真正写档案的位置，专门测名册锁会不会被中途放开。
type blockingProfileStore struct {
	base    *memoryProfileStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingProfileStore) Create(profile presets.Revision) error {
	return s.base.Create(profile)
}

func (s *blockingProfileStore) Get(id string) (presets.Revision, error) {
	return s.base.Get(id)
}

func (s *blockingProfileStore) GetRevision(id string, number int) (presets.Revision, error) {
	return s.base.GetRevision(id, number)
}

func (s *blockingProfileStore) List() ([]presets.Revision, error) {
	return s.base.List()
}

func (s *blockingProfileStore) Archive(id string) error {
	return s.base.Archive(id)
}

func (s *blockingProfileStore) Update(profile presets.Revision) error {
	close(s.entered)
	<-s.release
	return s.base.Update(profile)
}

// memoryProfileStore 是测试专用的档案库，语义与真正持久化档案库一致。
type memoryProfileStore struct {
	mu       sync.Mutex
	profiles map[string][]presets.Revision
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{profiles: make(map[string][]presets.Revision)}
}

func (s *memoryProfileStore) Create(profile presets.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.profiles[profile.ID]) > 0 {
		return fmt.Errorf("agent %s 已存在", profile.ID)
	}
	s.profiles[profile.ID] = []presets.Revision{cloneProfile(profile)}
	return nil
}

func (s *memoryProfileStore) Update(profile presets.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.profiles[profile.ID]
	if len(versions) == 0 {
		return fmt.Errorf("agent %s 不存在", profile.ID)
	}
	old := versions[len(versions)-1]
	if profile.Revision != old.Revision+1 {
		return fmt.Errorf("agent %s 版本不连续", profile.ID)
	}
	s.profiles[profile.ID] = append(versions, cloneProfile(profile))
	return nil
}

func (s *memoryProfileStore) Get(id string) (presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.profiles[id]
	if len(versions) == 0 {
		return presets.Revision{}, fmt.Errorf("agent %s 不存在", id)
	}
	return cloneProfile(versions[len(versions)-1]), nil
}

func (s *memoryProfileStore) GetRevision(id string, number int) (presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.profiles[id]
	if number < 1 || number > len(versions) {
		return presets.Revision{}, fmt.Errorf("agent %s 的版本 %d 不存在", id, number)
	}
	return cloneProfile(versions[number-1]), nil
}

func (s *memoryProfileStore) List() ([]presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles := make([]presets.Revision, 0, len(s.profiles))
	for _, versions := range s.profiles {
		profiles = append(profiles, cloneProfile(versions[len(versions)-1]))
	}
	return profiles, nil
}

func (s *memoryProfileStore) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.profiles[id]
	if len(versions) == 0 {
		return fmt.Errorf("agent %s 不存在", id)
	}
	profile := versions[len(versions)-1]
	profile.Archived = true
	profile.Revision++
	s.profiles[id] = append(versions, profile)
	return nil
}

func (memoryJournalPlugin) Name() string { return "test-memory-journal" }

func (p memoryJournalPlugin) Start(app *core.App) error {
	app.RegisterService("journal", p.journal)
	return nil
}

// scriptedAdapter 是剧本适配器：每次问模型按剧本回话，顺便记下收到的每个请求。
type scriptedAdapter struct {
	mu       sync.Mutex
	scripts  []scriptCall // 第 N 次请求用第 N 个剧本，超了报错
	requests []llm.Request
	block    chan struct{} // 非空时第一问会卡在这，用来制造"忙"
}

type scriptCall struct {
	deltas []string
	reply  llm.Reply
	hang   bool // true = 吐完字后挂住，等取消（测打断）
}

func (a *scriptedAdapter) Name() string {
	return "剧本"
}

func (a *scriptedAdapter) Stream(ctx context.Context, req llm.Request, onDelta func(chat.Delta)) (llm.Reply, error) {
	a.mu.Lock()
	index := len(a.requests)
	if index >= len(a.scripts) {
		a.mu.Unlock()
		return llm.Reply{}, llm.NewError("剧本", "bad_request", "剧本用完了")
	}
	script := a.scripts[index]
	a.requests = append(a.requests, req)
	block := a.block
	a.mu.Unlock()

	if block != nil && index == 0 {
		select {
		case <-block:
		case <-ctx.Done():
			return llm.Reply{}, llm.NewError("剧本", "cancelled", "被取消")
		}
	}

	text := ""
	for _, delta := range script.deltas {
		if onDelta != nil {
			onDelta(chat.Delta{Text: delta})
		}
		text += delta
	}

	if script.hang {
		select {
		case <-ctx.Done():
			return llm.Reply{}, llm.NewError("剧本", "cancelled", "用户打断了")
		case <-time.After(30 * time.Second):
		}
	}

	script.reply.Text = text
	return script.reply, nil
}

func (a *scriptedAdapter) sawRequests() []llm.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]llm.Request(nil), a.requests...)
}

// newTestStack 搭一套最小可跑的家当：core 场地 + 账本 + 插座排（剧本适配器）+ 工具 + loop。
func newTestStack(t *testing.T, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	return newTestStackWithStores(t, session.NewMemoryJournal(), newMemoryProfileStore(), scripts...)
}

func newTestStackWithJournal(t *testing.T, journal session.Journal, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	return newTestStackWithStores(t, journal, newMemoryProfileStore(), scripts...)
}

func newTestStackWithStores(t *testing.T, journal session.Journal, profiles ProfileStore, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	t.Helper()

	app := core.New()
	t.Cleanup(app.Close)

	projectStore := newMemoryProjectStore()
	err := app.Install(
		memoryProjectPlugin{store: projectStore},
		memoryProfilePlugin{store: profiles},
		memoryJournalPlugin{journal: journal},
		session.Plugin{},
		llm.Plugin{},
		tools.Plugin{},
		localenv.Plugin{},
		projects.Plugin{},
		presets.Plugin{},
	)
	if err != nil {
		t.Fatalf("装基础插件失败：%v", err)
	}

	adapter := &scriptedAdapter{scripts: scripts}
	llmSvc, err := llm.Get(app)
	if err != nil {
		t.Fatalf("取模型入口失败：%v", err)
	}
	err = llmSvc.Register(adapter)
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		t.Fatalf("取工具登记处失败：%v", err)
	}

	projectService, err := projects.Get(app)
	if err != nil {
		t.Fatalf("取项目管理入口失败：%v", err)
	}
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试项目失败：%v", err)
	}
	presetService, err := presets.Get(app)
	if err != nil {
		t.Fatalf("取模式管理入口失败：%v", err)
	}
	err = app.Install(agents.Plugin{}, Plugin{})
	if err != nil {
		t.Fatalf("装 loop 插件失败：%v", err)
	}
	service, err := agents.Get(app)
	if err != nil {
		t.Fatalf("取 Agent 管理入口失败：%v", err)
	}
	books, err := session.Get(app)
	if err != nil {
		t.Fatalf("取账本管家失败：%v", err)
	}
	roster := &testRoster{
		Service:   service,
		projects:  projectService,
		presets:   presetService,
		books:     books,
		projectID: project.ID,
		toolsReg:  registry,
		profiles:  make(map[string]llm.Selection),
	}

	return app, roster, adapter, registry
}

func startTestSession(t *testing.T, roster *Roster, id string, config RunConfig, seed ...session.Event) *Conversation {
	t.Helper()
	provider := config.Provider
	if provider == "" {
		provider = "剧本"
	}
	model := config.Model
	if model == "" {
		model = "test-model"
	}
	thinking := config.Thinking
	if thinking == "" {
		thinking = "off"
	}
	profile := AgentProfile{ID: id, Provider: provider, Model: model, Thinking: thinking, SystemPrompt: config.SystemPrompt, Tools: roster.toolsReg.Names()}
	err := roster.CreateAgent(profile)
	if err != nil {
		t.Fatalf("建 agent 档案失败：%v", err)
	}
	conversation, err := roster.StartSession(id, id, seed...)
	if err != nil {
		t.Fatalf("开会话失败：%v", err)
	}
	return conversation.(*Conversation)
}

func echoTool(registry *tools.Registry, t *testing.T) {
	err := registry.Register(tools.Tool{
		Schema: chat.ToolSchema{Name: "echo", Description: "回显", Parameters: []byte(`{"type":"object"}`)},
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			var args struct {
				Text string
			}
			err := json.Unmarshal(arguments, &args)
			if err != nil {
				return "", err
			}
			return args.Text, nil
		},
	})
	if err != nil {
		t.Fatalf("登记工具失败：%v", err)
	}
}

func kinds(events []session.Event) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.Kind
	}
	return out
}

func waitForState(t *testing.T, agent *Conversation, state string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for agent.State() != state && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if agent.State() != state {
		t.Fatalf("没等到 %s，当前 %s", state, agent.State())
	}
}

// event 手工造一笔旧账（seed 用）。
func event(kind string, seq int, data any) session.Event {
	raw, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return session.Event{Kind: kind, Seq: seq, Time: 1, Data: raw}
}
