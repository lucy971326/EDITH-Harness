package agents

import (
	"fmt"
	"sync"
	"testing"

	"harness/core"
	"harness/session"
	"harness/tools"
)

type memoryProfileStore struct {
	mu       sync.Mutex
	profiles map[string]AgentProfile
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{profiles: make(map[string]AgentProfile)}
}

func (s *memoryProfileStore) Create(profile AgentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.profiles[profile.ID]; exists {
		return fmt.Errorf("agent %s 已存在", profile.ID)
	}
	s.profiles[profile.ID] = cloneProfile(profile)
	return nil
}

func (s *memoryProfileStore) Update(profile AgentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, exists := s.profiles[profile.ID]
	if !exists {
		return fmt.Errorf("agent %s 不存在", profile.ID)
	}
	if profile.Revision != old.Revision+1 {
		return fmt.Errorf("agent %s 版本不连续", profile.ID)
	}
	s.profiles[profile.ID] = cloneProfile(profile)
	return nil
}

func (s *memoryProfileStore) Get(id string) (AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, exists := s.profiles[id]
	if !exists {
		return AgentProfile{}, fmt.Errorf("agent %s 不存在", id)
	}
	return cloneProfile(profile), nil
}

func (s *memoryProfileStore) List() ([]AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles := make([]AgentProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		profiles = append(profiles, cloneProfile(profile))
	}
	return profiles, nil
}

func (s *memoryProfileStore) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, exists := s.profiles[id]
	if !exists {
		return fmt.Errorf("agent %s 不存在", id)
	}
	profile.Archived = true
	profile.Revision++
	s.profiles[id] = profile
	return nil
}

type memoryProfilePlugin struct{ store ProfileStore }

func (memoryProfilePlugin) Name() string { return "test-memory-profiles" }

func (p memoryProfilePlugin) Start(app *core.App) error {
	app.RegisterService("agent-profiles", p.store)
	return nil
}

type memoryJournalPlugin struct{ journal session.Journal }

func (memoryJournalPlugin) Name() string { return "test-memory-journal" }

func (p memoryJournalPlugin) Start(app *core.App) error {
	app.RegisterService("journal", p.journal)
	return nil
}

type fakeRunner struct {
	input RunInput
}

func (r *fakeRunner) Prepare(input RunInput) (PreparedConversation, error) {
	r.input = input
	return &fakeConversation{input: input}, nil
}

type fakeConversation struct {
	input   RunInput
	started bool
}

func (c *fakeConversation) AgentID() string             { return c.input.AgentID }
func (c *fakeConversation) SessionID() string           { return c.input.SessionID }
func (c *fakeConversation) State() string               { return "idle" }
func (c *fakeConversation) WaitIdle()                   {}
func (c *fakeConversation) Cancel()                     {}
func (c *fakeConversation) SubmitFollowup(string) error { return nil }
func (c *fakeConversation) Steer(string) error          { return nil }
func (c *fakeConversation) InjectMemo(string) error     { return nil }
func (c *fakeConversation) Book() *session.Session      { return c.input.Book }
func (c *fakeConversation) Close() error                { return c.input.Close() }
func (c *fakeConversation) Start()                      { c.started = true }

func TestAgentsUsesRegisteredRunner(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(
		memoryProfilePlugin{store: newMemoryProfileStore()},
		memoryJournalPlugin{journal: session.NewMemoryJournal()},
		session.Plugin{},
		tools.Plugin{},
		Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	service, err := Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = service.CreateAgent(AgentProfile{ID: "小红", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.StartSession("小红", "会话一")
	if err == nil {
		t.Fatal("没装 Runner 时不该能开会话")
	}

	runner := &fakeRunner{}
	remove, err := service.SetRunner(runner)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := service.StartSession("小红", "会话一")
	if err != nil {
		t.Fatal(err)
	}
	if runner.input.AgentID != "小红" || runner.input.SessionID != "会话一" {
		t.Fatalf("Runner 收到的会话不对：%+v", runner.input)
	}
	if !conversation.(*fakeConversation).started {
		t.Fatal("登记完成后才该启动会话")
	}
	err = conversation.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.GetSession("会话一")
	if err == nil {
		t.Fatal("关闭后不该还能取到会话")
	}
	remove()
}
