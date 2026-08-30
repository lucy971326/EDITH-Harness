package runner

import (
	"context"
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
	mu     sync.Mutex
	cancel context.CancelFunc
	closed bool
	steers []session.UserMessage
}

// 活对象。挂在 Host 上、管理尚未结束 Run 的对话运行器。
type Runner struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	loops    loops.Loops
	events   *events.Registry

	mu   sync.Mutex
	live map[string]*liveRun
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

// Run 同步执行同一本 Session 的一轮对话。
func (r *Runner) Run(ctx context.Context, sessionID string, input session.UserMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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

	runCtx, cancel := context.WithCancel(ctx)
	current := &liveRun{cancel: cancel}
	err = r.begin(sessionID, current)
	if err != nil {
		cancel()
		return err
	}
	defer r.end(sessionID, current)

	message := messageFromInput(input)
	err = sess.Append(message)
	if err != nil {
		return err
	}
	err = r.publish(runCtx, sessionID, loops.Event{Kind: loops.EventMessage, Message: &message})
	if err != nil {
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
	if _, exists := r.live[sessionID]; exists {
		return fmt.Errorf("runner: session %q is already running", sessionID)
	}
	r.live[sessionID] = current
	return nil
}

func (r *Runner) end(sessionID string, current *liveRun) {
	current.mu.Lock()
	current.closed = true
	current.mu.Unlock()
	current.cancel()

	r.mu.Lock()
	if r.live[sessionID] == current {
		delete(r.live, sessionID)
	}
	r.mu.Unlock()
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
	}
	return r.publish(ctx, sessionID, event)
}

func (r *Runner) publish(ctx context.Context, sessionID string, event loops.Event) error {
	return events.Publish(ctx, r.events, RunEvent{SessionID: sessionID, Event: event})
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
		err = r.publish(ctx, sessionID, loops.Event{Kind: loops.EventMessage, Message: &message})
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (r *Runner) close() {
	r.mu.Lock()
	runs := make([]*liveRun, 0, len(r.live))
	for _, current := range r.live {
		runs = append(runs, current)
	}
	r.mu.Unlock()
	for _, current := range runs {
		current.cancel()
	}
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
