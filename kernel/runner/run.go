package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。一本 Session 当前尚未结束的一轮运行。
type liveRun struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	stopped         bool
	steeringClosed  bool
	followUpsClosed bool
	steers          []session.UserMessage
	followUps       []session.UserMessage
}

// 活对象。挂在 Host 上、管理尚未结束 Run 的对话运行器。
type Runner struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	loops    loops.Loops
	events   *events.Registry

	mu           sync.Mutex
	live         map[string]*liveRun
	closeStarted bool
	wg           sync.WaitGroup
}

// NewRunner 组装一份 Runner。
func NewRunner(
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	agentService *agents.Service,
	loopRegistry loops.Loops,
	eventRegistry *events.Registry,
) (*Runner, error) {
	if sessions == nil {
		return nil, fmt.Errorf("runner: nil sessions")
	}
	if settingsStore == nil {
		return nil, fmt.Errorf("runner: nil session settings")
	}
	if agentService == nil {
		return nil, fmt.Errorf("runner: nil agents")
	}
	if loopRegistry == nil {
		return nil, fmt.Errorf("runner: nil loops")
	}
	if eventRegistry == nil {
		return nil, fmt.Errorf("runner: nil events")
	}
	return &Runner{
		sessions: sessions,
		settings: settingsStore,
		agents:   agentService,
		loops:    loopRegistry,
		events:   eventRegistry,
		live:     make(map[string]*liveRun),
	}, nil
}

// Run 同步执行同一本 Session 的一轮对话，以及已排入的 FollowUp。
func (r *Runner) Run(ctx context.Context, sessionID string, input session.UserMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current := &liveRun{}
	if err := r.begin(sessionID, current); err != nil {
		return err
	}
	defer r.end(sessionID, current)
	return r.run(ctx, sessionID, input, current)
}

// Start 启动一轮对话并在 Runner 自己的 goroutine 中执行。
func (r *Runner) Start(ctx context.Context, sessionID string, input session.UserMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current := &liveRun{}
	if err := r.begin(sessionID, current); err != nil {
		return err
	}
	go func() {
		defer r.end(sessionID, current)
		_ = r.run(ctx, sessionID, input, current)
	}()
	return nil
}

func (r *Runner) run(ctx context.Context, sessionID string, input session.UserMessage, current *liveRun) error {
	next := input
	for {
		err := r.runOne(ctx, sessionID, next, current)
		if r.isClosing() {
			return err
		}
		followUp, ok := current.takeFollowUp()
		if !ok {
			return err
		}
		next = followUp
	}
}

func (r *Runner) runOne(parent context.Context, sessionID string, input session.UserMessage, current *liveRun) (err error) {
	current.openSteering()
	runCtx, cancel := context.WithCancel(parent)
	current.setCancel(cancel)
	defer cancel()

	err = r.publish(runCtx, RunEvent{SessionID: sessionID, Kind: RunStarted})
	if err != nil {
		return err
	}
	defer func() {
		status := RunSucceeded
		if errors.Is(err, context.Canceled) {
			status = RunCancelled
		} else if err != nil {
			status = RunFailed
		}
		endErr := r.publish(context.Background(), RunEvent{
			SessionID: sessionID,
			Kind:      RunEnded,
			Status:    status,
			Error:     errorText(err),
		})
		if err == nil && endErr != nil {
			err = endErr
		}
	}()

	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return err
	}
	runSettings, err := r.settings.For(sessionID)
	if err != nil {
		return err
	}
	prepared, err := r.agents.Prepare(runSettings.AgentID, runSettings.Workspace)
	if err != nil {
		return err
	}
	loop, err := r.loops.Get(prepared.Kind)
	if err != nil {
		return err
	}

	message := messageFromInput(input)
	if err = sess.Append(message); err != nil {
		return err
	}
	if err = r.publish(runCtx, RunEvent{SessionID: sessionID, Kind: Message, Message: &message}); err != nil {
		return err
	}

	invocation := loops.Invocation{
		History:      sess.History(),
		SystemPrompt: prepared.SystemPrompt,
		LLMConfig: llm.RunConfig{
			Model:           runSettings.Model,
			ReasoningEffort: runSettings.ReasoningEffort,
		},
		ToolNames: append([]string(nil), prepared.Tools...),
		Workspace: runSettings.Workspace,
		Emit: func(eventCtx context.Context, event loops.Event) error {
			return r.emit(eventCtx, sessionID, sess, event)
		},
		Checkpoint: func(checkpointCtx context.Context, phase loops.CheckpointPhase) ([]session.Message, error) {
			return r.checkpoint(checkpointCtx, sessionID, sess, current, phase)
		},
	}
	return loop.Run(runCtx, invocation)
}

func (r *Runner) begin(sessionID string, current *liveRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closeStarted {
		return fmt.Errorf("runner: closing")
	}
	if _, exists := r.live[sessionID]; exists {
		return fmt.Errorf("runner: session %q is already running", sessionID)
	}
	r.live[sessionID] = current
	r.wg.Add(1)
	return nil
}

func (r *Runner) end(sessionID string, current *liveRun) {
	current.mu.Lock()
	current.steeringClosed = true
	current.followUpsClosed = true
	cancel := current.cancel
	current.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	r.mu.Lock()
	if r.live[sessionID] == current {
		delete(r.live, sessionID)
	}
	r.mu.Unlock()
	r.wg.Done()
}

func (r *Runner) emit(ctx context.Context, sessionID string, sess *session.Session, event loops.Event) error {
	if event.Kind == loops.EventMessage {
		if event.Message == nil {
			return fmt.Errorf("runner: message event has nil message")
		}
		err := sess.Append(*event.Message)
		if err != nil {
			return err
		}
		return r.publish(ctx, RunEvent{SessionID: sessionID, Kind: Message, Message: event.Message})
	}
	runEvent, err := mapEvent(sessionID, event)
	if err != nil {
		return err
	}
	return r.publish(ctx, runEvent)
}

func (r *Runner) publish(ctx context.Context, event RunEvent) error {
	return events.Publish(ctx, r.events, event)
}

func (r *Runner) checkpoint(
	ctx context.Context,
	sessionID string,
	sess *session.Session,
	current *liveRun,
	phase loops.CheckpointPhase,
) ([]session.Message, error) {
	inputs, err := current.takeSteers(phase)
	if err != nil {
		return nil, err
	}
	messages := make([]session.Message, 0, len(inputs))
	for _, input := range inputs {
		message := messageFromInput(input)
		err = sess.Append(message)
		if err != nil {
			return nil, err
		}
		err = r.publish(ctx, RunEvent{SessionID: sessionID, Kind: Message, Message: &message})
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (r *Runner) close() {
	r.mu.Lock()
	r.closeStarted = true
	runs := make([]*liveRun, 0, len(r.live))
	for _, current := range r.live {
		runs = append(runs, current)
	}
	r.mu.Unlock()
	for _, current := range runs {
		current.stop()
	}
	r.wg.Wait()
}

func (r *Runner) isClosing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeStarted
}

func mapEvent(sessionID string, event loops.Event) (RunEvent, error) {
	out := RunEvent{SessionID: sessionID}
	switch event.Kind {
	case loops.EventTextDelta:
		out.Kind = TextDelta
		out.Text = event.Text
	case loops.EventReasoningDelta:
		out.Kind = ReasoningDelta
		out.Text = event.Text
	case loops.EventToolStarted:
		out.Kind = ToolStarted
	case loops.EventToolFinished:
		out.Kind = ToolFinished
	default:
		return RunEvent{}, fmt.Errorf("runner: unsupported loop event %q", event.Kind)
	}
	if event.Tool != nil {
		out.Tool = &ToolEvent{ID: event.Tool.ID, Name: event.Tool.Name, IsError: event.Tool.IsError}
	}
	return out, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func messageFromInput(input session.UserMessage) session.Message {
	return session.Message{Role: session.RoleUser, Blocks: cloneBlocks(input.Blocks)}
}

func cloneBlocks(blocks []session.Block) []session.Block {
	out := make([]session.Block, len(blocks))
	for i, block := range blocks {
		out[i] = block
		if block.Tool != nil {
			tool := *block.Tool
			out[i].Tool = &tool
		}
		if block.Result != nil {
			result := *block.Result
			out[i].Result = &result
		}
		if block.Media != nil {
			media := *block.Media
			out[i].Media = &media
		}
	}
	return out
}
