package agents

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"harness/core"
	"harness/environment"
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
func (*fakeConversation) InjectMemo(text string) error     { return nil }
func (c *fakeConversation) Book() *session.Session         { return c.input.Book }
func (c *fakeConversation) Close() error                   { return c.input.Close() }

type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

func newTestRegistry(t *testing.T) (*registry, projects.Service, presets.Service, *fakeRunner) {
	t.Helper()
	app := core.New()
	t.Cleanup(app.Close)
	books := session.NewStore(session.NewMemoryJournal(), silentBroadcaster{})
	projectService := projects.New(newProjectMemoryStore(), books)
	presetService := presets.New(newPresetMemoryStore())
	toolsReg := tools.NewRegistry()
	registry := newRegistry(app, books, projectService, presetService, environment.Provider(fakeEnvironment{}), toolsReg)
	runner := &fakeRunner{}
	_, err := registry.RegisterRunner(runner)
	if err != nil {
		t.Fatal(err)
	}
	return registry, projectService, presetService, runner
}

func TestSessionOwnsProjectPresetAndOpaqueID(t *testing.T) {
	registry, projectService, presetService, runner := newTestRegistry(t)
	project, err := projectService.Create("测试项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "极简", Provider: "fake", Model: "m1", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}

	conversation, err := registry.StartSession(StartInput{ProjectID: project.ID, PresetID: "极简", Title: "同名会话"})
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
	registry, projectService, presetService, runner := newTestRegistry(t)
	project, err := projectService.Create("测试项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "编程", Provider: "fake", Model: "old", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := registry.StartSession(StartInput{ProjectID: project.ID, PresetID: "编程", Title: "历史"})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := conversation.SessionID()
	err = conversation.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Update(presets.Preset{ID: "编程", Provider: "fake", Model: "new"})
	if err != nil {
		t.Fatal(err)
	}

	resumed, err := registry.ResumeSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.lastInput().Preset.Revision != 1 || runner.lastInput().Preset.Model != "old" {
		t.Fatalf("恢复时没有使用锁定版本：%+v", runner.lastInput().Preset)
	}
	_ = resumed.Close()
}

func TestConversationInterfaceDoesNotDependOnLoop(t *testing.T) {
	registry, projectService, presetService, _ := newTestRegistry(t)
	project, err := projectService.Create("测试项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "假模式", Provider: "fake", Model: "fake", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := registry.StartSession(StartInput{ProjectID: project.ID, PresetID: "假模式", Title: "假 Runner"})
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
	registry, projectService, presetService, runner := newTestRegistry(t)
	project, err := projectService.Create("测试项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "假模式", Provider: "fake", Model: "fake", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	provider := &failingEnvironment{}
	registry.environment = provider

	_, err = registry.StartSession(StartInput{ProjectID: project.ID, PresetID: "假模式", Title: "不会成功"})
	if err == nil {
		t.Fatal("环境挂载失败后会话却成功了")
	}
	if !provider.closed {
		t.Fatal("环境挂载失败后没有关闭会话作用域")
	}
	if len(registry.sessions) != 0 || len(registry.opening) != 0 {
		t.Fatalf("失败会话泄漏到运行表：sessions=%d opening=%d", len(registry.sessions), len(registry.opening))
	}
	if len(runner.inputs) != 0 {
		t.Fatal("环境挂载失败后不应交给 Runner")
	}
	headers, err := registry.books.ListHeaders()
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 0 {
		t.Fatal("环境挂载失败后不应创建账本")
	}
}
