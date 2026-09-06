package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/agents/config"
	"harness/kernel/events"
	"harness/kernel/llm"
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
	sessions    *session.Store
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
	return newRunnerFixtureWithLLM(t, loop, &llm.Client{})
}

func newRunnerFixtureWithLLM(t *testing.T, loop loops.Loop, client *llm.Client) runnerFixture {
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
	r, err := NewRunner(sessions, settingsStore, agentService, loopRegistry, eventRegistry, client, toolRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return runnerFixture{
		runner:      r,
		sessions:    sessions,
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

func TestRunPublishesUsageWithoutPersisting(t *testing.T) {
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		return invocation.Emit(ctx, loops.Event{
			Kind:  loops.EventUsage,
			Usage: &loops.Usage{InputTokens: 12, CacheReadTokens: 19, ContextWindow: 1000},
		})
	}}
	fixture := newRunnerFixture(t, loop)
	var got RunEvent
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == ContextUsage {
			got = event
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
	if got.Usage == nil || got.Usage.InputTokens != 12 || got.Usage.CacheReadTokens != 19 || got.Usage.ContextWindow != 1000 {
		t.Fatalf("usage event = %#v", got)
	}
	if len(fixture.session.History()) != 1 {
		t.Fatalf("history = %#v", fixture.session.History())
	}
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
	_, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
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

func TestStartReturnsStableHandleForFastCompletion(t *testing.T) {
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	if handle == nil || handle.RunID() == "" {
		t.Fatalf("handle = %#v", handle)
	}
	result := handle.Wait()
	if result.RunID != handle.RunID() || result.Status != RunSucceeded || result.Err != nil {
		t.Fatalf("result = %#v", result)
	}
	select {
	case <-handle.Done():
	default:
		t.Fatal("handle is not done")
	}
	_, ok := fixture.runner.State("session-1")
	if ok {
		t.Fatal("session remained live after handle completed")
	}
}

func TestStartReportsLoopFailureThroughHandle(t *testing.T) {
	loopErr := errors.New("loop failed")
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		return loopErr
	}})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, loopErr) {
		t.Fatalf("result = %#v", result)
	}
}

func TestStartReportsEarlyAppendFailureThroughHandle(t *testing.T) {
	appendErr := errors.New("disk failed")
	loopStartedErr := errors.New("loop started after append failure")
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		return loopStartedErr
	}})
	fixture.persistence.addFail = appendErr

	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, appendErr) {
		t.Fatalf("result = %#v", result)
	}
	_, ok := fixture.runner.State("session-1")
	if ok {
		t.Fatal("session remained live after early failure")
	}
}

func TestStartReportsInitialPublishFailureWithoutInventingRunEnded(t *testing.T) {
	publishErr := errors.New("subscriber failed")
	loopStartedErr := errors.New("loop started after publish failure")
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		return loopStartedErr
	}})
	var kinds []RunEventKind
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		kinds = append(kinds, event.Kind)
		if event.Kind == Message {
			return publishErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, publishErr) {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(kinds, []RunEventKind{Message}) {
		t.Fatalf("events = %v", kinds)
	}
}

func TestStartPairsRunStartedPublishFailureWithRunEnded(t *testing.T) {
	publishErr := errors.New("start subscriber failed")
	endErr := errors.New("end subscriber failed")
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	var observed []RunEventKind
	var delivered []RunEventKind
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		observed = append(observed, event.Kind)
		if event.Kind == RunStarted {
			return publishErr
		}
		if event.Kind == RunEnded {
			return endErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		delivered = append(delivered, event.Kind)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, publishErr) || !errors.Is(result.Err, endErr) {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(observed, []RunEventKind{Message, RunStarted, RunEnded}) {
		t.Fatalf("events = %v", observed)
	}
	if !reflect.DeepEqual(delivered, []RunEventKind{Message, RunStarted, RunEnded}) {
		t.Fatalf("successful subscriber events = %v", delivered)
	}
}

func TestRunEndedPublishFailureIsNotSwallowed(t *testing.T) {
	publishErr := errors.New("end subscriber failed")
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == RunEnded {
			return publishErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, publishErr) {
		t.Fatalf("result = %#v", result)
	}
}

func TestStartStopRaceReportsCancellation(t *testing.T) {
	signals := make(chan (<-chan struct{}), 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		if invocation.InputSignal == nil {
			return errors.New("missing input signal")
		}
		signals <- invocation.InputSignal()
		<-ctx.Done()
		return ctx.Err()
	}})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunCancelled || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result = %#v", result)
	}
	select {
	case signal := <-signals:
		select {
		case <-signal:
			t.Fatal("Stop produced an input signal")
		default:
		}
	default:
	}
}

func TestInitialMessageStopDoesNotReopenSteering(t *testing.T) {
	messageEntered := make(chan struct{})
	releaseMessage := make(chan struct{})
	steerAttempted := make(chan error, 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == Message && event.Entry != nil && len(event.Entry.Message.Blocks) > 0 && event.Entry.Message.Blocks[0].Text == "initial" {
			close(messageEntered)
			<-releaseMessage
		}
		if event.Kind == RunStarted {
			steerAttempted <- fixture.runner.Steer("session-1", textInput("must not enter"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("initial"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-messageEntered:
	case <-time.After(time.Second):
		t.Fatal("initial Message event did not block")
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Steer("session-1", textInput("must not enter while stopped"))
	if err == nil || !strings.Contains(err.Error(), "not running") {
		close(releaseMessage)
		t.Fatalf("Steer after Stop error = %v", err)
	}
	close(releaseMessage)
	result := handle.Wait()
	if result.Status != RunCancelled || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result = %#v", result)
	}
	select {
	case err = <-steerAttempted:
		t.Fatalf("RunStarted was published and accepted Steer: %v", err)
	default:
	}
	history := fixture.session.History()
	if len(history) != 1 || history[0].Blocks[0].Text != "initial" {
		t.Fatalf("history after cancelled startup = %#v", history)
	}
}

func TestInitialMessageCloseDoesNotReopenSteering(t *testing.T) {
	messageEntered := make(chan struct{})
	releaseMessage := make(chan struct{})
	steerAttempted := make(chan error, 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == Message && event.Entry != nil && len(event.Entry.Message.Blocks) > 0 && event.Entry.Message.Blocks[0].Text == "initial" {
			close(messageEntered)
			<-releaseMessage
		}
		if event.Kind == RunStarted {
			steerAttempted <- fixture.runner.Steer("session-1", textInput("must not enter"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("initial"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-messageEntered:
	case <-time.After(time.Second):
		t.Fatal("initial Message event did not block")
	}
	current, err := fixture.runner.current("session-1")
	if err != nil {
		t.Fatal(err)
	}
	closeDone := make(chan struct{})
	go func() {
		fixture.runner.close()
		close(closeDone)
	}()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		current.mu.Lock()
		closed := current.steeringState == steeringClosed
		current.mu.Unlock()
		if closed {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("Runner.Close did not close steering")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	err = fixture.runner.Steer("session-1", textInput("must not enter while closed"))
	if err == nil || !strings.Contains(err.Error(), "not running") {
		close(releaseMessage)
		t.Fatalf("Steer after Close cancellation error = %v", err)
	}
	close(releaseMessage)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Runner.Close did not finish")
	}
	result := handle.Wait()
	if result.Status != RunCancelled || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result = %#v", result)
	}
	select {
	case err = <-steerAttempted:
		t.Fatalf("RunStarted was published and accepted Steer: %v", err)
	default:
	}
	history := fixture.session.History()
	if len(history) != 1 || history[0].Blocks[0].Text != "initial" {
		t.Fatalf("history after closed startup = %#v", history)
	}
}

func TestCloseWaitsUntilStartedHandleIsDone(t *testing.T) {
	started := make(chan struct{})
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	fixture.runner.close()
	select {
	case <-handle.Done():
	default:
		t.Fatal("Runner.Close returned before the handle completed")
	}
	result := handle.Wait()
	if result.Status != RunCancelled || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunSettingsUsesLiveRunSnapshot(t *testing.T) {
	started := make(chan struct{})
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}})
	original := fixture.settings.value
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	err = fixture.settings.Put("session-1", settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "model-b",
		ReasoningEffort: "low",
		Workspace:       "/workspace/b",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.runner.RunSettings("session-1", handle.RunID())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != original {
		t.Fatalf("snapshot = %#v, want %#v", snapshot, original)
	}
	got := handle.Settings()
	if got != original {
		t.Fatalf("handle settings = %#v, want %#v", got, original)
	}
	_, err = fixture.runner.RunSettings("session-1", "other-run")
	if err == nil {
		t.Fatal("RunSettings accepted a different run id")
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunCancelled {
		t.Fatalf("result = %#v", result)
	}
	_, err = fixture.runner.RunSettings("session-1", handle.RunID())
	if err == nil {
		t.Fatal("RunSettings returned a snapshot after the run ended")
	}
}

func TestRunPassesProgramIdentityToInvocation(t *testing.T) {
	invocationSeen := make(chan loops.Invocation, 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(_ context.Context, invocation loops.Invocation) error {
		invocationSeen <- invocation
		return nil
	}})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunSucceeded {
		t.Fatalf("result = %#v", result)
	}
	select {
	case invocation := <-invocationSeen:
		if invocation.SessionID != "session-1" || invocation.RunID != handle.RunID() {
			t.Fatalf("invocation identity = %q/%q, want %q/%q", invocation.SessionID, invocation.RunID, "session-1", handle.RunID())
		}
		if invocation.InputSignal == nil {
			t.Fatal("invocation has no input signal")
		}
	case <-time.After(time.Second):
		t.Fatal("loop did not receive invocation")
	}
}

func TestSteerInputSignalStaysVisibleUntilCheckpointConsumesSteer(t *testing.T) {
	signalReady := make(chan (<-chan struct{}), 1)
	secondSignalReady := make(chan (<-chan struct{}), 1)
	releaseCheckpoint := make(chan struct{})
	finishRun := make(chan struct{})
	checkpointMessages := make(chan []session.Message, 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		if invocation.InputSignal == nil {
			return errors.New("missing input signal")
		}
		signalReady <- invocation.InputSignal()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-releaseCheckpoint:
		}
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		if err != nil {
			return err
		}
		checkpointMessages <- messages
		secondSignalReady <- invocation.InputSignal()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-finishRun:
			return nil
		}
	}})
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	var signal <-chan struct{}
	select {
	case signal = <-signalReady:
	case <-time.After(time.Second):
		t.Fatal("loop did not expose input signal")
	}
	err = fixture.runner.Steer("session-1", textInput("steer"))
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Steer("session-1", textInput("second steer"))
	if err != nil {
		t.Fatal(err)
	}
	for observation := 0; observation < 2; observation++ {
		select {
		case <-signal:
		default:
			t.Fatalf("Steer signal was not visible on observation %d", observation+1)
		}
	}
	close(releaseCheckpoint)
	select {
	case messages := <-checkpointMessages:
		if len(messages) != 2 || messages[0].Blocks[0].Text != "steer" || messages[1].Blocks[0].Text != "second steer" {
			t.Fatalf("checkpoint messages = %#v", messages)
		}
	case <-time.After(time.Second):
		t.Fatal("checkpoint did not consume Steer")
	}
	var secondSignal <-chan struct{}
	select {
	case secondSignal = <-secondSignalReady:
	case <-time.After(time.Second):
		t.Fatal("loop did not expose the next input signal")
	}
	select {
	case <-secondSignal:
		t.Fatal("checkpoint left a pending input signal")
	default:
	}
	err = fixture.runner.Steer("session-1", textInput("after checkpoint"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondSignal:
	case <-time.After(time.Second):
		t.Fatal("next Steer did not wake the next signal generation")
	}
	close(finishRun)
	result := handle.Wait()
	if result.Status != RunSucceeded || result.Err != nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunInitialHistoryPrecedesAcceptedSteer(t *testing.T) {
	invocationSeen := make(chan loops.Invocation, 1)
	checkpointMessages := make(chan []session.Message, 1)
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		invocationSeen <- invocation
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		if err != nil {
			return err
		}
		checkpointMessages <- messages
		return nil
	}})
	var steerErr error
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind != RunStarted {
			return nil
		}
		steerErr = fixture.runner.Steer("session-1", textInput("steer after start"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("initial"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunSucceeded || result.Err != nil {
		t.Fatalf("result = %#v", result)
	}
	if steerErr != nil {
		t.Fatalf("RunStarted subscriber Steer error = %v", steerErr)
	}
	select {
	case invocation := <-invocationSeen:
		if len(invocation.History) != 1 || invocation.History[0].Blocks[0].Text != "initial" {
			t.Fatalf("initial history = %#v", invocation.History)
		}
	case <-time.After(time.Second):
		t.Fatal("loop did not receive invocation")
	}
	select {
	case messages := <-checkpointMessages:
		if len(messages) != 1 || messages[0].Blocks[0].Text != "steer after start" {
			t.Fatalf("checkpoint messages = %#v", messages)
		}
	case <-time.After(time.Second):
		t.Fatal("checkpoint did not consume Steer")
	}
}

func TestRunResultKeepsWrappedJoinedFailureFailed(t *testing.T) {
	diskErr := errors.New("disk failed")
	err := fmt.Errorf("outer: %w", errors.Join(context.Canceled, diskErr))
	result := runResult("run-1", err)
	if result.Status != RunFailed {
		t.Fatalf("status = %s, want %s", result.Status, RunFailed)
	}
	if !errors.Is(result.Err, diskErr) {
		t.Fatalf("result error = %v, want disk error", result.Err)
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
	_, err = fixture.runner.Start(context.Background(), "session-1", textInput("question"))
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
