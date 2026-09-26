package conversations_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness/internal/agents"
	"harness/internal/approvals"
	"harness/internal/appserver"
	"harness/internal/commands"
	"harness/internal/conversations"
	"harness/internal/events"
	"harness/internal/llm"
	"harness/internal/loops"
	machinelocal "harness/internal/machine/local"
	"harness/internal/persist"
	"harness/internal/runner"
	"harness/internal/session"
	"harness/internal/session/settings"
	"harness/internal/skills"
	"harness/internal/subagents"
	"harness/internal/tools"

	"github.com/coder/websocket"
)

func TestProductRunsWithoutWebAndForksCompletedSegment(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()

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
	created.Settings.PermissionMode = "read_only"
	_, err = fixture.service.UpdateSettings(t.Context(), created.Meta.ID, created.Settings)
	if err != nil {
		t.Fatal(err)
	}

	eventsSeen := make(chan runner.RunEvent, 16)
	unsubscribe, err := events.Subscribe(fixture.events, func(_ context.Context, event runner.RunEvent) error {
		eventsSeen <- event
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	err = fixture.service.Start(context.Background(), conversations.RunInput{
		SessionID: created.Meta.ID, Model: "deepseek/deepseek-flash", ReasoningEffort: "high",
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "first"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	invocation := fixture.loop.waitStarted(t)
	steerDone := make(chan error, 1)
	go func() {
		steerDone <- fixture.service.Steer(created.Meta.ID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "steer"}}})
	}()
	select {
	case <-invocation.InputSignal():
	case <-time.After(time.Second):
		t.Fatal("Steer did not reach Runner")
	}
	fixture.loop.release()
	if err = <-steerDone; err != nil {
		t.Fatal(err)
	}
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
	if len(snapshot.Runs) != 1 || snapshot.Runs[0].Status != runner.RunSucceeded {
		t.Fatalf("snapshot runs = %#v json=%s", snapshot.Runs, encoded)
	}

	forkID, err := fixture.service.Fork(conversations.ForkInput{SessionID: created.Meta.ID, RunID: snapshot.Entries[1].Message.RunID, BoundaryEntryID: snapshot.Entries[1].ID})
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
	if setup.Workspace != workspace || setup.Model != "deepseek/deepseek-flash" || setup.ReasoningEffort != "high" || setup.PermissionMode != "read_only" {
		t.Fatalf("fork settings = %#v", setup)
	}

	stopping, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.Start(context.Background(), conversations.RunInput{
		SessionID: stopping.Meta.ID, Model: "deepseek/deepseek-flash", ReasoningEffort: "high",
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

func TestProductCreateDiscardsSessionWhenSettingsSaveFails(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	service, err := conversations.New(fixture.sessions, failingSettings{store: fixture.settings}, fixture.agents, fixture.models, fixture.runner, fixture.commands, fixture.subagents, approvals.New())
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

func TestProductCreateSelectsConfiguredModel(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()

	cases := []struct {
		name       string
		config     string
		wantModel  string
		wantFailed bool
	}{
		{name: "skip empty key", config: "providers:\n  deepseek:\n    apiKey: \"\"\n  google:\n    apiKey: test-key\n", wantModel: "google/gemini-3.5-flash-lite"},
		{name: "no usable key", config: "providers:\n  deepseek:\n    apiKey: \"\"\n", wantFailed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := persist.NewFiles(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			err = files.Write("config.yaml", []byte(tc.config))
			if err != nil {
				t.Fatal(err)
			}
			models, err := llm.New(files)
			if err != nil {
				t.Fatal(err)
			}
			service, err := conversations.New(fixture.sessions, fixture.settings, fixture.agents, models, fixture.runner, fixture.commands, fixture.subagents, approvals.New())
			if err != nil {
				t.Fatal(err)
			}
			before, err := fixture.sessions.List()
			if err != nil {
				t.Fatal(err)
			}
			created, err := service.Create(t.TempDir())
			if tc.wantFailed {
				if err == nil {
					t.Fatal("want missing key error")
				}
				after, listErr := fixture.sessions.List()
				if listErr != nil {
					t.Fatal(listErr)
				}
				if len(after) != len(before) {
					t.Fatalf("failed create left session: before=%d after=%d", len(before), len(after))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if created.Settings.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", created.Settings.Model, tc.wantModel)
			}
		})
	}
}

func TestProductSessionDoesNotReadOtherSessionSettings(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
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
	service, err := conversations.New(fixture.sessions, selectiveFailSettings{store: fixture.settings, badID: "bad"}, fixture.agents, fixture.models, fixture.runner, fixture.commands, fixture.subagents, approvals.New())
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
	close     func() error
	service   *conversations.Service
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
	err = os.WriteFile(filepath.Join(home, ".harness", "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n  google:\n    apiKey: test-key\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	previousHome := os.Getenv("HOME")
	err = os.Setenv("HOME", home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", previousHome) })

	loop := newTestLoop()
	files, err := persist.NewFiles(filepath.Join(home, ".harness"))
	if err != nil {
		t.Fatal(err)
	}
	disk := session.NewPersistence(files)
	settingsStore := settings.NewStore(files)
	sessions := session.NewStore(disk)
	models, err := llm.New(files)
	if err != nil {
		t.Fatal(err)
	}
	machineService, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = machineService.Close() })
	eventRegistry := events.NewRegistry()
	loopRegistry := loops.NewRegistry()
	toolRegistry := tools.NewRegistry()
	skillService := skills.NewRegistry()
	err = loopRegistry.Register(loop)
	if err != nil {
		t.Fatal(err)
	}
	agentService, err := agents.NewService(agents.NewStore(files), settingsStore, loopRegistry, toolRegistry, skillService)
	if err != nil {
		t.Fatal(err)
	}
	runService, err := runner.NewRunner(sessions, settingsStore, agentService, loopRegistry, eventRegistry, models, toolRegistry, files, machineService)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runService.Close)
	subFiles, err := files.Scope("subagents")
	if err != nil {
		t.Fatal(err)
	}
	subagentService, err := subagents.NewSubagents(sessions, settingsStore, agentService, models, runService, eventRegistry, subFiles)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subagentService.Close() })
	closeServices := func() error {
		err := subagentService.Close()
		runService.Close()
		return errors.Join(err, machineService.Close())
	}
	commandService := commands.NewRegistry()
	approvalService, err := approvals.Open(files, models, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = approvalService.Close() })
	service, err := conversations.New(sessions, settingsStore, agentService, models, runService, commandService, subagentService, approvalService)
	if err != nil {
		t.Fatal(err)
	}
	return testFixture{close: closeServices, service: service, sessions: sessions, settings: settingsStore, agents: agentService, models: models, runner: runService, commands: commandService, events: eventRegistry, subagents: subagentService, loop: loop}
}

type testLoop struct {
	started   chan loops.Invocation
	releaseCh chan struct{}
}

func newTestLoop() *testLoop {
	return &testLoop{started: make(chan loops.Invocation, 8), releaseCh: make(chan struct{}, 8)}
}

func (l *testLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "test"}
}

func (l *testLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	l.started <- invocation
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

func (l *testLoop) waitStarted(t *testing.T) loops.Invocation {
	t.Helper()
	select {
	case invocation := <-l.started:
		return invocation
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
		return loops.Invocation{}
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

func TestSubagentsChatIsolation(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()

	workspace := t.TempDir()
	created, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.service.Start(context.Background(), conversations.RunInput{
		SessionID: created.Meta.ID, Model: "deepseek/deepseek-flash", ReasoningEffort: "high",
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

	spawnRes, err := fixture.subagents.Spawn(context.Background(), subagents.SpawnInput{TaskName: "test",
		ParentSessionID: created.Meta.ID,
		ParentRunID:     state.RunID,
		Description:     "isolated child",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	fixture.loop.release()

	// 同样的隔离必须经过真实接口分发成立，而不只测直接调用。
	server := newRPCServer(t, fixture)
	defer server.Close()
	params, err := json.Marshal(appserver.SessionIDParams{SessionID: spawnRes.ChildSessionID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(context.Background(), "harness/session/get", params)
	assertMethodError(t, err, appserver.CodeNotFound)
	raw, err := server.Call(context.Background(), "harness/session/list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var wireList appserver.ListResult
	err = json.Unmarshal(raw, &wireList)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range wireList.Sessions {
		if item.SessionID == spawnRes.ChildSessionID {
			t.Fatal("child leaked through interface")
		}
	}
	params, err = json.Marshal(appserver.CreateParams{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = server.Call(context.Background(), "harness/session/create", params)
	if err != nil {
		t.Fatal(err)
	}
	var wireCreated appserver.SessionResult
	err = json.Unmarshal(raw, &wireCreated)
	if err != nil {
		t.Fatal(err)
	}
	if wireCreated.Session.SessionID == spawnRes.ChildSessionID {
		t.Fatal("interface reused child")
	}

	// 1. HarnessProduct.List 绝不包含子会话
	chatList, err := fixture.service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range chatList {
		if item.Meta.ID == spawnRes.ChildSessionID {
			t.Fatalf("child session %q leaked into chat list", spawnRes.ChildSessionID)
		}
	}

	// 2. HarnessProduct.Create 绝不复用空子会话
	newChat, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if newChat.Meta.ID == spawnRes.ChildSessionID {
		t.Fatalf("child session %q was reused by conversations.Create", spawnRes.ChildSessionID)
	}

	// 3. HarnessProduct.Session 拒绝访问子会话
	_, err = fixture.service.Session(spawnRes.ChildSessionID)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
	_, err = fixture.service.Snapshot(spawnRes.ChildSessionID)
	if !errors.Is(err, conversations.ErrSessionNotFound) {
		t.Fatalf("snapshot exposed child session: %v", err)
	}
	err = fixture.service.Stop(spawnRes.ChildSessionID)
	if !errors.Is(err, conversations.ErrSessionNotFound) {
		t.Fatalf("stop exposed child session: %v", err)
	}

	// 4. HarnessProduct.Start / Steer / CallCommand 拒绝操作子会话
	err = fixture.service.Start(context.Background(), conversations.RunInput{
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
	err = fixture.service.CallCommand(context.Background(), "compact", spawnRes.ChildSessionID)
	if !errors.Is(err, conversations.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestNestedSubagentInterfacesUseDirectParent(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()

	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.Start(t.Context(), conversations.RunInput{
		SessionID: created.Meta.ID, Model: "deepseek/deepseek-flash", ReasoningEffort: "high",
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "root"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := fixture.loop.waitStarted(t)
	child, err := fixture.subagents.Spawn(t.Context(), subagents.SpawnInput{
		TaskName: "child", ParentSessionID: created.Meta.ID, ParentRunID: root.RunID, Description: "delegate",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	grandchild, err := fixture.subagents.Spawn(t.Context(), subagents.SpawnInput{
		TaskName: "grandchild", ParentSessionID: child.ChildSessionID, ParentRunID: child.RunID, Description: "nested delegate",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)

	tasks, err := fixture.service.SubagentList(child.ChildSessionID)
	if err != nil || len(tasks) != 1 || tasks[0].TaskID != grandchild.TaskID {
		t.Fatalf("nested list = %+v, %v", tasks, err)
	}
	snapshot, err := fixture.service.SubagentSnapshot(child.ChildSessionID, grandchild.TaskID)
	if err != nil || snapshot.ChildSessionID != grandchild.ChildSessionID {
		t.Fatalf("nested snapshot = %+v, %v", snapshot, err)
	}
	_, err = fixture.service.SubagentSnapshot(created.Meta.ID, grandchild.TaskID)
	if err == nil {
		t.Fatalf("wrong direct parent accessed grandchild: %v", err)
	}
	for _, childSessionID := range []string{child.ChildSessionID, grandchild.ChildSessionID} {
		_, err = fixture.service.Session(childSessionID)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ordinary session API exposed %q: %v", childSessionID, err)
		}
	}

	// 真实 WebSocket 订阅必须带回下一层面板继续导航所需的孩子 Session ID。
	server := newRPCServer(t, fixture)
	defer server.Close()
	baseURL, err := server.Listen("127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(baseURL, "http")+"/rpc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	writeSocketRequest(t, ws, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)
	readSocketResponse(t, ws)
	params, err := json.Marshal(appserver.SubagentParams{ParentSessionID: child.ChildSessionID, TaskID: grandchild.TaskID})
	if err != nil {
		t.Fatal(err)
	}
	writeSocketRequest(t, ws, string(mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "harness/subagent/subscribe", "params": json.RawMessage(params),
	})))
	response := readSocketResponse(t, ws)
	if response.Error != nil {
		t.Fatalf("nested subscribe: %+v", response.Error)
	}
	var subscribed appserver.SubagentSubscribeResult
	if err := json.Unmarshal(response.Result, &subscribed); err != nil {
		t.Fatal(err)
	}
	if subscribed.ChildSessionID != grandchild.ChildSessionID {
		t.Fatalf("subscribe childSessionID = %q, want %q", subscribed.ChildSessionID, grandchild.ChildSessionID)
	}

	err = fixture.service.StopSubagent(t.Context(), child.ChildSessionID, grandchild.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	waitIdle(t, fixture.runner, grandchild.ChildSessionID)
	updated, err := fixture.service.UpdateSubagentSettings(t.Context(), child.ChildSessionID, grandchild.TaskID, "deepseek/deepseek-flash", "high")
	if err != nil || updated.Model != "deepseek/deepseek-flash" {
		t.Fatalf("nested settings = %+v, %v", updated, err)
	}
	sent, err := fixture.service.SendSubagent(t.Context(), child.ChildSessionID, grandchild.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "more work"}},
	})
	if err != nil || sent.Steered {
		t.Fatalf("nested send = %+v, %v", sent, err)
	}
	fixture.loop.waitStarted(t)
}

type socketRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeSocketRequest(t *testing.T, ws *websocket.Conn, raw string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := ws.Write(ctx, websocket.MessageText, []byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func readSocketResponse(t *testing.T, ws *websocket.Conn) socketRPCResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var response socketRPCResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// 单个会话的慢命令不能挡住其他会话；同会话协调由同一操作锁负责。
func TestCommandDoesNotBlockOtherSession(t *testing.T) {
	f := newTestFixture(t)
	defer f.close()
	first, err := f.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	command := &blockingCommand{sessionID: first.Meta.ID, started: make(chan struct{}), release: make(chan struct{})}
	err = f.commands.Register(command)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.service.CallCommand(context.Background(), "block", first.Meta.ID) }()
	defer func() { close(command.release); <-done }()
	select {
	case <-command.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first command did not start")
	}
	other := make(chan error, 1)
	go func() { other <- f.service.CallCommand(context.Background(), "block", second.Meta.ID) }()
	select {
	case err := <-other:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated session blocked")
	}
}

type blockingCommand struct {
	sessionID string
	started   chan struct{}
	release   chan struct{}
}

func (*blockingCommand) Name() string        { return "block" }
func (*blockingCommand) Description() string { return "block one session" }
func (c *blockingCommand) Run(_ context.Context, sessionID string) error {
	if sessionID == c.sessionID {
		close(c.started)
		<-c.release
	}
	return nil
}
