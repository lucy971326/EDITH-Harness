package runner

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/agents/config"
	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

type runnerTestLoop struct {
	run func(context.Context, loops.Invocation) error
}

func (l *runnerTestLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "runner test loop"}
}

func (l *runnerTestLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	return l.run(ctx, invocation)
}

type memoryPersistence struct {
	mu      sync.Mutex
	trees   map[string][]persist.Node
	metas   map[string]persist.Meta
	addFail error
}

func newMemoryPersistence() *memoryPersistence {
	return &memoryPersistence{trees: make(map[string][]persist.Node), metas: make(map[string]persist.Meta)}
}

func (p *memoryPersistence) Load(id string) (*persist.Tree, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	nodes, ok := p.trees[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &persist.Tree{ID: id, Nodes: append([]persist.Node(nil), nodes...)}, nil
}

func (p *memoryPersistence) Save(id string, tree *persist.Tree) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trees[id] = append([]persist.Node(nil), tree.Nodes...)
	return nil
}

func (p *memoryPersistence) LoadMeta(id string) (persist.Meta, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	meta, ok := p.metas[id]
	if !ok {
		return persist.Meta{}, os.ErrNotExist
	}
	return meta, nil
}

func (p *memoryPersistence) SaveMeta(meta persist.Meta) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metas[meta.ID] = meta
	return nil
}

func (p *memoryPersistence) DeleteMeta(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.metas[id]; !ok {
		return os.ErrNotExist
	}
	delete(p.metas, id)
	return nil
}

func (p *memoryPersistence) List() ([]persist.Meta, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]persist.Meta, 0, len(p.metas))
	for _, meta := range p.metas {
		out = append(out, meta)
	}
	return out, nil
}

func (p *memoryPersistence) Add(id string, node persist.Node) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.addFail != nil {
		return p.addFail
	}
	p.trees[id] = append(p.trees[id], node)
	return nil
}

type memorySettings struct {
	mu    sync.Mutex
	value settings.SessionSettings
	reads int
}

func (s *memorySettings) For(string) (settings.SessionSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return s.value, nil
}

func (s *memorySettings) Put(_ string, value settings.SessionSettings) error {
	s.mu.Lock()
	s.value = value
	s.mu.Unlock()
	return nil
}

func (s *memorySettings) UsesAgent(agentID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value.AgentID == agentID, nil
}

func (s *memorySettings) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

type emptyAgentStore struct {
	agents map[string]config.Agent
}

func newEmptyAgentStore() *emptyAgentStore {
	return &emptyAgentStore{agents: make(map[string]config.Agent)}
}

func (s *emptyAgentStore) ListAgents() ([]config.Agent, error) {
	out := make([]config.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		out = append(out, agent)
	}
	return out, nil
}

func (s *emptyAgentStore) ForAgent(id string) (config.Agent, error) {
	agent, ok := s.agents[id]
	if !ok {
		return config.Agent{}, os.ErrNotExist
	}
	return agent, nil
}

func (s *emptyAgentStore) PutAgent(agent config.Agent) error {
	s.agents[agent.ID] = agent
	return nil
}

func (s *emptyAgentStore) DeleteAgent(id string) error {
	delete(s.agents, id)
	return nil
}

type runnerFixture struct {
	runner      *Runner
	session     *session.Session
	persistence *memoryPersistence
	settings    *memorySettings
	events      *events.Registry
	agents      *agents.Service
	tools       *tools.Registry
	skills      *skills.Registry
}

func newRunnerFixture(t *testing.T, loop loops.Loop) runnerFixture {
	t.Helper()
	persistence := newMemoryPersistence()
	sessions := session.NewStore(persistence)
	sess, err := sessions.Create("session-1")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore := &memorySettings{value: settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "model-a",
		ReasoningEffort: "high",
		Workspace:       "/workspace/a",
	}}
	loopRegistry := loops.NewRegistry()
	err = loopRegistry.Register(loop)
	if err != nil {
		t.Fatal(err)
	}
	toolRegistry := tools.NewRegistry()
	skillRegistry := skills.NewRegistry()
	agentService, err := agents.NewService(
		newEmptyAgentStore(),
		settingsStore,
		loopRegistry,
		toolRegistry,
		skillRegistry,
	)
	if err != nil {
		t.Fatal(err)
	}
	eventRegistry := events.NewRegistry()
	r, err := NewRunner(sessions, settingsStore, agentService, loopRegistry, eventRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return runnerFixture{
		runner:      r,
		session:     sess,
		persistence: persistence,
		settings:    settingsStore,
		events:      eventRegistry,
		agents:      agentService,
		tools:       toolRegistry,
		skills:      skillRegistry,
	}
}

func textInput(text string) session.UserMessage {
	return session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: text}}}
}

type testRunnerSkillErrorProvider struct {
	err error
}

func (testRunnerSkillErrorProvider) Name() string { return "error" }

func (p testRunnerSkillErrorProvider) List(string) ([]skills.Skill, error) {
	return nil, p.err
}

func TestRunBuildsInvocationPersistsMessagesAndPublishesInOrder(t *testing.T) {
	var gotInvocation loops.Invocation
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		gotInvocation = invocation
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, Text: "hello"})
		if err != nil {
			return err
		}
		assistant := session.Message{
			Role:   session.RoleAssistant,
			Blocks: []session.Block{{Kind: "text", Text: "hello"}},
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &assistant})
		if err != nil {
			return err
		}
		toolResult := session.Message{
			Role: session.RoleTool,
			Blocks: []session.Block{{
				Kind:   "tool-result",
				Result: &session.ToolResult{ID: "call-1", Name: "read", Content: "done"},
			}},
		}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &toolResult})
	}}
	fixture := newRunnerFixture(t, loop)

	var kinds []RunEventKind
	var durableLengths []int
	var ended RunEvent
	var runID string
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.SessionID != "session-1" {
			t.Fatalf("session id = %q", event.SessionID)
		}
		if event.RunID == "" {
			t.Fatal("run event has no run id")
		}
		if runID == "" {
			runID = event.RunID
		} else if event.RunID != runID {
			t.Fatalf("run id = %q, want %q", event.RunID, runID)
		}
		kinds = append(kinds, event.Kind)
		if event.Kind == Message {
			durableLengths = append(durableLengths, len(fixture.session.History()))
		}
		if event.Kind == RunEnded {
			ended = event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.runner.Run(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}

	if fixture.settings.readCount() != 1 {
		t.Fatalf("settings reads = %d", fixture.settings.readCount())
	}
	if len(gotInvocation.History) != 1 || gotInvocation.History[0].Role != session.RoleUser {
		t.Fatalf("initial history = %#v", gotInvocation.History)
	}
	if gotInvocation.LLMConfig.Model != "model-a" || gotInvocation.LLMConfig.ReasoningEffort != "high" {
		t.Fatalf("llm config = %#v", gotInvocation.LLMConfig)
	}
	if gotInvocation.Workspace != "/workspace/a" {
		t.Fatalf("workspace = %q", gotInvocation.Workspace)
	}
	if !strings.Contains(gotInvocation.SystemPrompt, "/workspace/a") {
		t.Fatalf("system prompt = %q", gotInvocation.SystemPrompt)
	}
	wantKinds := []RunEventKind{
		Message,
		RunStarted,
		TextDelta,
		Message,
		Message,
		RunEnded,
	}
	if kinds[0] != Message || kinds[1] != RunStarted {
		t.Fatalf("initial events = %v", kinds[:2])
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("event kinds = %v", kinds)
	}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("event kinds = %v", kinds)
		}
	}
	if ended.Status != RunSucceeded || ended.Error != "" {
		t.Fatalf("run ended = %#v", ended)
	}
	if len(durableLengths) != 3 || durableLengths[0] != 1 || durableLengths[1] != 2 || durableLengths[2] != 3 {
		t.Fatalf("durable lengths = %v", durableLengths)
	}
	history := fixture.session.History()
	if len(history) != 3 {
		t.Fatalf("history length = %d", len(history))
	}
	if history[0].Role != session.RoleUser || history[1].Role != session.RoleAssistant || history[2].Role != session.RoleTool {
		t.Fatalf("history roles = %v, %v, %v", history[0].Role, history[1].Role, history[2].Role)
	}
	for _, message := range history {
		if message.RunID != runID {
			t.Fatalf("message run id = %q, want %q", message.RunID, runID)
		}
	}
}

func TestRunStopsAfterPublishedDurableMessageFails(t *testing.T) {
	publishErr := errors.New("subscriber failed")
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		assistant := session.Message{
			Role:   session.RoleAssistant,
			Blocks: []session.Block{{Kind: "text", Text: "answer"}},
		}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &assistant})
	}}
	fixture := newRunnerFixture(t, loop)
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Entry != nil && event.Entry.Message.Role == session.RoleAssistant {
			return publishErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.runner.Run(context.Background(), "session-1", textInput("question"))
	if !errors.Is(err, publishErr) {
		t.Fatalf("error = %v", err)
	}
	history := fixture.session.History()
	if len(history) != 2 || history[1].Role != session.RoleAssistant {
		t.Fatalf("history = %#v", history)
	}
}

func TestRunAnchorsToolEventsToOriginalAssistantBlock(t *testing.T) {
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		assistant := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{
			{Kind: "reasoning", Text: "inspect"},
			{Kind: "tool-call", Tool: &session.ToolCall{ID: "call-1", Name: "read", Args: `{}`}},
		}}
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, StepSeq: 1, Message: &assistant})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventToolStarted, StepSeq: 1, BlockSeq: 2, Tool: &loops.ToolEvent{ID: "call-1", Name: "read"}})
		if err != nil {
			return err
		}
		result := session.Message{Role: session.RoleTool, Blocks: []session.Block{{Kind: "tool-result", Result: &session.ToolResult{ID: "call-1", Name: "read", Content: "done"}}}}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, StepSeq: 1, BlockSeq: 2, Message: &result})
	}}
	fixture := newRunnerFixture(t, loop)
	var started, result RunEvent
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == ToolStarted {
			started = event
		}
		if event.Kind == Message && event.Entry != nil && event.Entry.Message.Role == session.RoleTool {
			result = event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Run(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	if started.AfterEntrySeq != 1 || started.StepSeq != 1 || started.BlockSeq != 2 {
		t.Fatalf("tool start position = %#v", started)
	}
	if result.AfterEntrySeq != 1 || result.StepSeq != 1 || result.BlockSeq != 2 {
		t.Fatalf("tool result position = %#v", result)
	}
}

func TestRunKeepsInitialAnchorAfterSteer(t *testing.T) {
	checkpointed := make(chan struct{})
	continueRun := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		_, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		if err != nil {
			return err
		}
		close(checkpointed)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-continueRun:
		}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, StepSeq: 2, BlockSeq: 1, Text: "continued"})
	}}
	fixture := newRunnerFixture(t, loop)
	var afterEntrySeq uint64
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == TextDelta {
			afterEntrySeq = event.AfterEntrySeq
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() {
		runDone <- fixture.runner.Run(context.Background(), "session-1", textInput("question"))
	}()
	<-checkpointed
	err = fixture.runner.Steer("session-1", textInput("steer"))
	if err != nil {
		t.Fatal(err)
	}
	close(continueRun)
	select {
	case err = <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
	if afterEntrySeq != 1 {
		t.Fatalf("run anchor after steer = %d, want 1", afterEntrySeq)
	}
}

func TestStateReportsLiveRunAfterInputIsDurable(t *testing.T) {
	entered := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	fixture := newRunnerFixture(t, loop)
	err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	state, ok := fixture.runner.State("session-1")
	if !ok || state.RunID == "" || state.AfterEntrySeq != 1 {
		t.Fatalf("live state = %#v, %t", state, ok)
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStartReturnsSkillDiscoveryErrorBeforeStartingLoop(t *testing.T) {
	loopCalled := false
	loop := &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		loopCalled = true
		return nil
	}}
	fixture := newRunnerFixture(t, loop)
	err := fixture.tools.Register(tools.New("read", "read", func(context.Context, tools.Call, struct{}) (tools.Result, error) {
		return tools.Result{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.agents.Save(agents.Agent{
		ID: agents.DefaultID, Name: "Harness", Kind: "react", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.skills.Register(testRunnerSkillErrorProvider{err: errors.New("skill discovery failed")})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err == nil || !strings.Contains(err.Error(), "skill discovery failed") {
		t.Fatalf("Start() error = %v", err)
	}
	if loopCalled {
		t.Fatal("loop started after preparation failed")
	}
	if _, ok := fixture.runner.State("session-1"); ok {
		t.Fatal("session remained live after preparation failed")
	}
	if len(fixture.session.History()) != 0 {
		t.Fatalf("history = %#v, want empty", fixture.session.History())
	}
}

func TestRunDoesNotPublishMessageThatFailedToAppend(t *testing.T) {
	loopCalled := false
	loop := &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		loopCalled = true
		return nil
	}}
	fixture := newRunnerFixture(t, loop)
	appendErr := errors.New("disk failed")
	fixture.persistence.addFail = appendErr
	publishedMessage := false
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		publishedMessage = publishedMessage || event.Kind == Message
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.runner.Run(context.Background(), "session-1", textInput("question"))
	if !errors.Is(err, appendErr) {
		t.Fatalf("error = %v", err)
	}
	if publishedMessage {
		t.Fatal("published a message that was not appended")
	}
	if loopCalled {
		t.Fatal("loop was called")
	}
}

func TestRunReadsNewSettingsOnlyOnTheNextRun(t *testing.T) {
	var invocations []loops.Invocation
	var fixture runnerFixture
	loop := &runnerTestLoop{run: func(_ context.Context, invocation loops.Invocation) error {
		invocations = append(invocations, invocation)
		if len(invocations) == 1 {
			return fixture.settings.Put("session-1", settings.SessionSettings{
				AgentID:         agents.DefaultID,
				Model:           "model-b",
				ReasoningEffort: "low",
				Workspace:       "/workspace/b",
			})
		}
		return nil
	}}
	fixture = newRunnerFixture(t, loop)

	err := fixture.runner.Run(context.Background(), "session-1", textInput("first"))
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Run(context.Background(), "session-1", textInput("second"))
	if err != nil {
		t.Fatal(err)
	}

	if len(invocations) != 2 {
		t.Fatalf("invocations = %d", len(invocations))
	}
	if invocations[0].LLMConfig.Model != "model-a" || invocations[0].Workspace != "/workspace/a" {
		t.Fatalf("first invocation = %#v", invocations[0])
	}
	if invocations[1].LLMConfig.Model != "model-b" || invocations[1].Workspace != "/workspace/b" {
		t.Fatalf("second invocation = %#v", invocations[1])
	}
}
