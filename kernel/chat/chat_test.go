package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/subagents"
	"harness/kernel/tools"
)

func TestServiceRunsWithoutWebAndForksCompletedSegment(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()

	workspace := t.TempDir()
	created, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if reused.Meta.ID != created.Meta.ID {
		t.Fatalf("empty session was not reused: %q != %q", reused.Meta.ID, created.Meta.ID)
	}

	eventsSeen := make(chan runner.RunEvent, 16)
	unsubscribe, err := fixture.service.SubscribeRun(func(_ context.Context, event runner.RunEvent) error {
		eventsSeen <- event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	err = fixture.service.Start(context.Background(), RunInput{
		SessionID: created.Meta.ID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high",
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "first"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	err = fixture.service.Steer(created.Meta.ID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "steer"}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.release()
	waitEnded(t, eventsSeen, created.Meta.ID)

	snapshot, err := fixture.service.Snapshot(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 3 || snapshot.Entries[0].Message.Blocks[0].Text != "first" || snapshot.Entries[1].Message.Blocks[0].Text != "steer" || snapshot.Entries[2].Message.Blocks[0].Text != "completed" {
		t.Fatalf("history = %#v", snapshot.Entries)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || !containsRunsArray(string(encoded)) {
		t.Fatalf("snapshot JSON = %s", encoded)
	}

	forkID, err := fixture.service.Fork(ForkInput{SessionID: created.Meta.ID, RunID: snapshot.Entries[1].Message.RunID, BoundaryEntryID: snapshot.Entries[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := fixture.service.Snapshot(forkID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fork.Entries) != 3 {
		t.Fatalf("fork history length = %d", len(fork.Entries))
	}
	setup, err := fixture.settings.For(forkID)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Workspace != workspace || setup.Model != "deepseek/deepseek-v4-flash" || setup.ReasoningEffort != "high" {
		t.Fatalf("fork settings = %#v", setup)
	}

	stopping, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.Start(context.Background(), RunInput{
		SessionID: stopping.Meta.ID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high",
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "stop"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	err = fixture.service.Stop(stopping.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitEnded(t, eventsSeen, stopping.Meta.ID)
}

func TestServiceCreateDiscardsSessionWhenSettingsSaveFails(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	service, err := NewService(fixture.sessions, failingSettings{store: fixture.settings}, fixture.agents, fixture.models, fixture.runner, fixture.commands, fixture.events, fixture.subagents)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(t.TempDir())
	if err == nil {
		t.Fatal("want settings failure")
	}
	all, err := fixture.sessions.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("sessions = %#v", all)
	}
}

func TestServiceSessionDoesNotReadOtherSessionSettings(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	workspace := t.TempDir()
	good, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.sessions.Create("bad")
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.settings.Put("bad", settings.SessionSettings{AgentID: agents.DefaultID, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(fixture.sessions, selectiveFailSettings{store: fixture.settings, badID: "bad"}, fixture.agents, fixture.models, fixture.runner, fixture.commands, fixture.events, fixture.subagents)
	if err != nil {
		t.Fatal(err)
	}
	info, err := service.Session(good.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Meta.ID != good.Meta.ID {
		t.Fatalf("session = %#v", info)
	}
}

type testFixture struct {
	host      *host.Host
	service   *Service
	sessions  *session.Store
	settings  settings.SessionSettingsStore
	agents    *agents.Service
	models    *llm.Client
	runner    *runner.Runner
	commands  commands.Commands
	events    *events.Registry
	subagents *subagents.Subagents
	loop      *testLoop
}

func newTestFixture(t *testing.T) testFixture {
	t.Helper()
	home := t.TempDir()
	err := os.Mkdir(filepath.Join(home, ".harness"), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(home, ".harness", "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	previousHome := os.Getenv("HOME")
	err = os.Setenv("HOME", home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", previousHome) })

	h := host.NewHost()
	plugins := []host.Plugin{&persist.Plugin{Dir: t.TempDir()}, &session.Plugin{}, &llm.Plugin{}, events.NewPlugin(), loops.NewPlugin(), skills.NewPlugin(), tools.NewPlugin(), agents.NewPlugin(), commands.NewPlugin(), runner.NewPlugin(), subagents.NewPlugin(t.TempDir()), NewPlugin()}
	for _, plugin := range plugins {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
		if plugin.Name() == "loops" {
			registry, resolveErr := host.Resolve[loops.Loops](h, "loops")
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			loop := newTestLoop()
			registerErr := registry.Register(loop)
			if registerErr != nil {
				t.Fatal(registerErr)
			}
		}
	}
	service, err := host.Resolve[*Service](h, "chatService")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	agentService, err := host.Resolve[*agents.Service](h, "agents")
	if err != nil {
		t.Fatal(err)
	}
	models, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		t.Fatal(err)
	}
	runService, err := host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		t.Fatal(err)
	}
	commandService, err := host.Resolve[commands.Commands](h, "commands")
	if err != nil {
		t.Fatal(err)
	}
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		t.Fatal(err)
	}
	subagentService, err := host.Resolve[*subagents.Subagents](h, "subagents")
	if err != nil {
		t.Fatal(err)
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		t.Fatal(err)
	}
	registeredLoop, err := loopRegistry.Get("react")
	if err != nil {
		t.Fatal(err)
	}
	loop, ok := registeredLoop.(*testLoop)
	if !ok {
		t.Fatal("test loop unavailable")
	}
	return testFixture{host: h, service: service, sessions: sessions, settings: settingsStore, agents: agentService, models: models, runner: runService, commands: commandService, events: eventRegistry, subagents: subagentService, loop: loop}
}

type testLoop struct {
	started   chan struct{}
	releaseCh chan struct{}
}

func newTestLoop() *testLoop {
	return &testLoop{started: make(chan struct{}, 8), releaseCh: make(chan struct{}, 8)}
}

func (l *testLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "test"}
}

func (l *testLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	l.started <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.releaseCh:
	}
	steers, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
	if err != nil {
		return err
	}
	text := "completed"
	if len(steers) == 0 {
		text = "completed without steer"
	}
	message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: text}}}
	return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &message})
}

func (l *testLoop) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-l.started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
}

func (l *testLoop) release() { l.releaseCh <- struct{}{} }

type failingSettings struct{ store settings.SessionSettingsStore }

func (f failingSettings) For(id string) (settings.SessionSettings, error) { return f.store.For(id) }
func (f failingSettings) Put(string, settings.SessionSettings) error      { return errors.New("disk full") }
func (f failingSettings) UsesAgent(id string) (bool, error)               { return f.store.UsesAgent(id) }

type selectiveFailSettings struct {
	store settings.SessionSettingsStore
	badID string
}

func (f selectiveFailSettings) For(id string) (settings.SessionSettings, error) {
	if id == f.badID {
		return settings.SessionSettings{}, errors.New("broken settings")
	}
	return f.store.For(id)
}
func (f selectiveFailSettings) Put(id string, value settings.SessionSettings) error {
	return f.store.Put(id, value)
}
func (f selectiveFailSettings) UsesAgent(id string) (bool, error) { return f.store.UsesAgent(id) }

func waitEnded(t *testing.T, received <-chan runner.RunEvent, sessionID string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-received:
			if event.SessionID == sessionID && event.Kind == runner.RunEnded {
				return
			}
		case <-deadline:
			t.Fatal("run did not end")
		}
	}
}

func containsRunsArray(value string) bool { return strings.Contains(value, "\"runs\":[]") }

func TestSubagentsChatIsolation(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()

	workspace := t.TempDir()
	created, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.service.Start(context.Background(), RunInput{
		SessionID: created.Meta.ID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high",
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "parent prompt"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	defer fixture.loop.release()

	state, ok := fixture.runner.State(created.Meta.ID)
	if !ok {
		t.Fatal("expected parent running")
	}

	spawnRes, err := fixture.subagents.Spawn(context.Background(), subagents.SpawnInput{
		ParentSessionID: created.Meta.ID,
		ParentRunID:     state.RunID,
		Description:     "isolated child",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	fixture.loop.release()

	// 1. ChatService.List 绝不包含子会话
	chatList, err := fixture.service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range chatList {
		if item.Meta.ID == spawnRes.ChildSessionID {
			t.Fatalf("child session %q leaked into chat list", spawnRes.ChildSessionID)
		}
	}

	// 2. ChatService.Create 绝不复用空子会话
	newChat, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if newChat.Meta.ID == spawnRes.ChildSessionID {
		t.Fatalf("child session %q was reused by chatService.Create", spawnRes.ChildSessionID)
	}

	// 3. ChatService.Session 拒绝访问子会话
	_, err = fixture.service.Session(spawnRes.ChildSessionID)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// 4. ChatService.Start / Steer 拒绝操作子会话
	err = fixture.service.Start(context.Background(), RunInput{
		SessionID: spawnRes.ChildSessionID,
		Message:   session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "hi"}}},
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
	err = fixture.service.Steer(spawnRes.ChildSessionID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "steer"}},
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}
