package agents

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"harness/chat"
	"harness/core"
	"harness/environment"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

type projectMemoryStore struct {
	mu    sync.Mutex
	items map[string]projects.Project
}

func newProjectMemoryStore() *projectMemoryStore {
	return &projectMemoryStore{items: make(map[string]projects.Project)}
}

func (s *projectMemoryStore) Create(project projects.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[project.ID]; exists {
		return fmt.Errorf("项目已存在")
	}
	s.items[project.ID] = project
	return nil
}

func (s *projectMemoryStore) Get(id string) (projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, exists := s.items[id]
	if !exists {
		return projects.Project{}, fmt.Errorf("项目不存在")
	}
	return project, nil
}

func (s *projectMemoryStore) List() ([]projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := make([]projects.Project, 0, len(s.items))
	for _, project := range s.items {
		listed = append(listed, project)
	}
	return listed, nil
}

func (s *projectMemoryStore) Update(project projects.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[project.ID] = project
	return nil
}

type presetMemoryStore struct {
	mu       sync.Mutex
	versions map[string][]presets.Revision
}

func newPresetMemoryStore() *presetMemoryStore {
	return &presetMemoryStore{versions: make(map[string][]presets.Revision)}
}

func (s *presetMemoryStore) Create(revision presets.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.versions[revision.ID]) > 0 {
		return fmt.Errorf("模式已存在")
	}
	s.versions[revision.ID] = []presets.Revision{revision}
	return nil
}

func (s *presetMemoryStore) Update(revision presets.Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[revision.ID] = append(s.versions[revision.ID], revision)
	return nil
}

func (s *presetMemoryStore) Get(id string) (presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.versions[id]
	if len(versions) == 0 {
		return presets.Revision{}, fmt.Errorf("模式不存在")
	}
	return versions[len(versions)-1], nil
}

func (s *presetMemoryStore) GetRevision(id string, number int) (presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.versions[id]
	if number < 1 || number > len(versions) {
		return presets.Revision{}, fmt.Errorf("版本不存在")
	}
	return versions[number-1], nil
}

func (s *presetMemoryStore) List() ([]presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := make([]presets.Revision, 0, len(s.versions))
	for _, versions := range s.versions {
		listed = append(listed, versions[len(versions)-1])
	}
	sort.Slice(listed, func(i int, j int) bool { return listed[i].ID < listed[j].ID })
	return listed, nil
}

func (s *presetMemoryStore) Archive(id string) error {
	current, err := s.Get(id)
	if err != nil {
		return err
	}
	current.Revision++
	current.Archived = true
	return s.Update(current)
}

type fakeEnvironment struct{}

func (fakeEnvironment) Mount(scope *core.App, root string) error {
	scope.RegisterService("test-root", root)
	return nil
}

type failingEnvironment struct {
	closed bool
}

func (e *failingEnvironment) Mount(scope *core.App, root string) error {
	scope.OnCleanup(func() {
		e.closed = true
	})
	return fmt.Errorf("环境坏了")
}

type fakeRunner struct {
	mu     sync.Mutex
	inputs []RunInput
}

func (r *fakeRunner) Prepare(input RunInput) (PreparedConversation, error) {
	r.mu.Lock()
	r.inputs = append(r.inputs, input)
	r.mu.Unlock()
	return &fakeConversation{input: input}, nil
}

func (r *fakeRunner) lastInput() RunInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inputs[len(r.inputs)-1]
}

type fakeConversation struct {
	input RunInput
}

func (*fakeConversation) Start()                           {}
func (c *fakeConversation) SessionID() string              { return c.input.SessionID }
func (*fakeConversation) State() string                    { return "idle" }
func (*fakeConversation) WaitIdle()                        {}
func (*fakeConversation) Cancel()                          {}
func (*fakeConversation) SubmitFollowup(text string) error { return nil }
func (*fakeConversation) Steer(text string) error          { return nil }
func (*fakeConversation) SelectModel(llm.Selection) error  { return nil }
func (c *fakeConversation) Book() *session.Session         { return c.input.Book }
func (c *fakeConversation) Close() error                   { return c.input.Close() }

type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

func newTestConversationManager(t *testing.T) (*conversationManager, projects.Service, presets.Service, *fakeRunner) {
	t.Helper()
	app := core.New()
	t.Cleanup(app.Close)
	books := session.NewStore(session.NewMemoryJournal(), silentBroadcaster{})
	projectService := projects.New(newProjectMemoryStore(), books)
	presetService := presets.New(newPresetMemoryStore())
	toolsReg := tools.NewRegistry()
	llmService := llm.NewService()
	err := llmService.Register(testAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	manager := newConversationManager(app, books, projectService, presetService, environment.Provider(fakeEnvironment{}), toolsReg, llmService)
	runner := &fakeRunner{}
	_, err = manager.RegisterRunner(runner)
	if err != nil {
		t.Fatal(err)
	}
	return manager, projectService, presetService, runner
}

type testAdapter struct{}

func (testAdapter) Name() string { return "test" }

func (testAdapter) Stream(context.Context, llm.Request, func(chat.Delta)) (llm.Reply, error) {
	return llm.Reply{}, nil
}

func (testAdapter) ProviderInfo() llm.ProviderInfo {
	return llm.ProviderInfo{Name: "test", Models: []llm.ModelInfo{{ID: "test", ThinkingLevels: []string{"off"}}}}
}

func TestSessionOwnsProjectPresetAndOpaqueID(t *testing.T) {
	manager, projectService, presetService, runner := newTestConversationManager(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "极简"})
	if err != nil {
		t.Fatal(err)
	}

	conversation, err := manager.StartSession(StartInput{ProjectID: project.ID, PresetID: "极简", Title: "同名会话"})
	if err != nil {
		t.Fatal(err)
	}
	header := conversation.Book().Header()
	if header.ID == header.Title || header.Title != "同名会话" {
		t.Fatalf("内部身份和标题没有分开：%+v", header)
	}
	if header.ProjectID != project.ID || header.PresetID != "极简" || header.PresetRevision != 1 {
		t.Fatalf("封面没有锁住项目和模式：%+v", header)
	}
	root, err := core.Resolve[string](runner.lastInput().Scope, "test-root")
	if err != nil || root != project.Root {
		t.Fatalf("会话没有挂到项目根目录：root=%q err=%v", root, err)
	}
	listed, err := projectService.ListSessions(project.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != header.ID {
		t.Fatalf("项目没有领到自己的会话：%+v err=%v", listed, err)
	}
	err = conversation.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestResumeUsesLockedPresetRevision(t *testing.T) {
	manager, projectService, presetService, runner := newTestConversationManager(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "编程", SystemPrompt: "旧提示词"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := manager.StartSession(StartInput{ProjectID: project.ID, PresetID: "编程", Title: "历史"})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := conversation.SessionID()
	err = conversation.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Update(presets.Preset{ID: "编程", SystemPrompt: "新提示词"})
	if err != nil {
		t.Fatal(err)
	}

	resumed, err := manager.ResumeSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.lastInput().Preset.Revision != 1 || runner.lastInput().Preset.SystemPrompt != "旧提示词" {
		t.Fatalf("恢复时没有使用锁定版本：%+v", runner.lastInput().Preset)
	}
	_ = resumed.Close()
}

func TestOpenSessionUsesRunningConversationOrRestoresHistory(t *testing.T) {
	manager, projectService, presetService, runner := newTestConversationManager(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartSession(StartInput{ProjectID: project.ID, PresetID: "模式", Title: "历史"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := manager.OpenSession(started.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if opened != started || len(runner.inputs) != 1 {
		t.Fatal("运行中的会话应直接返回，不该重复恢复")
	}
	err = started.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.OpenSession(started.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.inputs) != 2 || !runner.lastInput().Recover {
		t.Fatal("历史会话应恢复一次后返回")
	}
}

func TestConversationInterfaceDoesNotDependOnLoop(t *testing.T) {
	manager, projectService, presetService, _ := newTestConversationManager(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "假模式"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := manager.StartSession(StartInput{ProjectID: project.ID, PresetID: "假模式", Title: "假 Runner"})
	if err != nil {
		t.Fatal(err)
	}
	err = conversation.SubmitFollowup("你好")
	if err != nil {
		t.Fatal(err)
	}
	conversation.Cancel()
	conversation.WaitIdle()
	_ = conversation.Close()
}

func TestMountFailurePublishesNoSessionAndClosesScope(t *testing.T) {
	manager, projectService, presetService, runner := newTestConversationManager(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "假模式"})
	if err != nil {
		t.Fatal(err)
	}
	provider := &failingEnvironment{}
	manager.environment = provider

	_, err = manager.StartSession(StartInput{ProjectID: project.ID, PresetID: "假模式", Title: "不会成功"})
	if err == nil {
		t.Fatal("环境挂载失败后会话却成功了")
	}
	if !provider.closed {
		t.Fatal("环境挂载失败后没有关闭会话作用域")
	}
	if len(manager.sessions) != 0 || len(manager.opening) != 0 {
		t.Fatalf("失败会话泄漏到运行表：sessions=%d opening=%d", len(manager.sessions), len(manager.opening))
	}
	if len(runner.inputs) != 0 {
		t.Fatal("环境挂载失败后不应交给 Runner")
	}
	headers, err := manager.books.ListHeaders()
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 0 {
		t.Fatal("环境挂载失败后不应创建账本")
	}
}
