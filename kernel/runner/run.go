package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/tools"
)

// 数据。一轮运行接收 Steer 的阶段；关闭后不再重新开放。
type steeringState uint8

const (
	steeringInitializing steeringState = iota
	steeringOpen
	steeringClosed
)

// 活对象。一本 Session 当前尚未结束的一轮运行。
type liveRun struct {
	mu                sync.Mutex
	cancel            context.CancelFunc
	steeringState     steeringState
	steers            []session.Message
	inputErr          error
	inputPublications sync.WaitGroup
	afterEntrySeq     uint64
	runID             string
	settings          settings.SessionSettings
	// 当前代次的通道在有待处理 Steer 时关闭广播；Checkpoint 消费后才换代次。
	inputSignal chan struct{}
	toolBlocks  map[string]toolBlock
}

// 数据。一轮 Run 在进入 Loop 前读取的一致配置快照。
type runPreparation struct {
	sess     *session.Session
	settings settings.SessionSettings
	prepared agents.PreparedAgent
	loop     loops.Loop
}

// 数据。一场 Run 内工具调用所在的原始块位置。
type toolBlock struct {
	stepSeq  uint64
	blockSeq uint64
}

// 活对象。挂在 Host 上、管理尚未结束 Run 的对话运行器。
type Runner struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	loops    loops.Loops
	events   *events.Registry
	llm      *llm.Client
	tools    tools.Tools

	mu           sync.Mutex
	live         map[string]*liveRun
	closeStarted bool
	wg           sync.WaitGroup
}

// 活对象。一场异步 Run 的身份、配置和完成通知句柄。
type RunHandle struct {
	runID    string
	settings settings.SessionSettings
	done     chan struct{}
	result   RunResult
	once     sync.Once
}

// RunID 返回这场 Run 的稳定身份。
func (h *RunHandle) RunID() string { return h.runID }

// Done 返回 Run 完成时关闭的信号。
func (h *RunHandle) Done() <-chan struct{} { return h.done }

// Wait 等待 Run 完成并返回最终结果。
func (h *RunHandle) Wait() RunResult {
	<-h.done
	return h.result
}

// Settings 返回启动时保存的本轮配置快照。
func (h *RunHandle) Settings() settings.SessionSettings { return h.settings }

func newRunHandle(runID string, runSettings settings.SessionSettings) *RunHandle {
	return &RunHandle{
		runID:    runID,
		settings: runSettings,
		done:     make(chan struct{}),
	}
}

func (h *RunHandle) complete(result RunResult) {
	h.once.Do(func() {
		h.result = result
		close(h.done)
	})
}

// NewRunner 组装一份 Runner。
func NewRunner(
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	agentService *agents.Service,
	loopRegistry loops.Loops,
	eventRegistry *events.Registry,
	llmClient *llm.Client,
	toolRegistry tools.Tools,
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
	if llmClient == nil {
		return nil, fmt.Errorf("runner: nil llm")
	}
	if toolRegistry == nil {
		return nil, fmt.Errorf("runner: nil tools")
	}
	return &Runner{
		sessions: sessions,
		settings: settingsStore,
		agents:   agentService,
		loops:    loopRegistry,
		events:   eventRegistry,
		llm:      llmClient,
		tools:    toolRegistry,
		live:     make(map[string]*liveRun),
	}, nil
}

// Run 同步执行同一本 Session 的一轮对话。
func (r *Runner) Run(ctx context.Context, sessionID string, input session.UserMessage) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared, err := r.prepare(ctx, sessionID)
	if err != nil {
		return err
	}
	handle, current, runCtx, err := r.openRun(ctx, sessionID, prepared.settings)
	if err != nil {
		return err
	}
	defer func() {
		r.release(sessionID, current)
		handle.complete(runResult(handle.RunID(), err))
		r.wg.Done()
	}()
	return r.executePrepared(runCtx, sessionID, handle.RunID(), input, current, prepared)
}

// Start 启动一轮对话并在 Runner 自己的 goroutine 中执行。
func (r *Runner) Start(ctx context.Context, sessionID string, input session.UserMessage) (*RunHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := r.prepare(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	handle, current, runCtx, err := r.openRun(ctx, sessionID, prepared.settings)
	if err != nil {
		return nil, err
	}
	go func() {
		var runErr error
		defer func() {
			r.release(sessionID, current)
			handle.complete(runResult(handle.RunID(), runErr))
			r.wg.Done()
		}()
		runErr = r.executePrepared(runCtx, sessionID, handle.RunID(), input, current, prepared)
	}()
	return handle, nil
}

func (r *Runner) openRun(ctx context.Context, sessionID string, runSettings settings.SessionSettings) (*RunHandle, *liveRun, context.Context, error) {
	runID, current, runCtx, err := r.openLive(ctx, sessionID, runSettings)
	if err != nil {
		return nil, nil, nil, err
	}
	return newRunHandle(runID, runSettings), current, runCtx, nil
}

func (r *Runner) openLive(ctx context.Context, sessionID string, runSettings settings.SessionSettings) (string, *liveRun, context.Context, error) {
	runID, err := newRunID()
	if err != nil {
		return "", nil, nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	current := &liveRun{
		cancel:        cancel,
		steeringState: steeringInitializing,
		runID:         runID,
		settings:      runSettings,
		inputSignal:   make(chan struct{}),
		toolBlocks:    make(map[string]toolBlock),
	}
	err = r.begin(sessionID, current)
	if err != nil {
		cancel()
		return "", nil, nil, err
	}
	return runID, current, runCtx, nil
}

func (r *Runner) prepare(ctx context.Context, sessionID string) (runPreparation, error) {
	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return runPreparation{}, err
	}
	runSettings, err := r.settings.For(sessionID)
	if err != nil {
		return runPreparation{}, err
	}
	prepared, err := r.agents.Prepare(ctx, runSettings.AgentID, runSettings.Workspace)
	if err != nil {
		return runPreparation{}, err
	}
	loop, err := r.loops.Get(prepared.Kind)
	if err != nil {
		return runPreparation{}, err
	}
	return runPreparation{sess: sess, settings: runSettings, prepared: prepared, loop: loop}, nil
}

func (r *Runner) executePrepared(runCtx context.Context, sessionID, runID string, input session.UserMessage, current *liveRun, preparation runPreparation) (err error) {
	sess := preparation.sess
	runSettings := preparation.settings
	prepared := preparation.prepared
	loop := preparation.loop
	runStartedAttempted := false
	defer func() {
		// 不再接收协作输入后，等待已落账输入的事件发布结束，保留迟到的发布错误。
		current.closeSteering()
		current.inputPublications.Wait()
		current.mu.Lock()
		err = errors.Join(err, current.inputErr)
		current.mu.Unlock()
		if !runStartedAttempted {
			return
		}
		status := RunSucceeded
		if isCancellationOnly(err) {
			status = RunCancelled
		} else if err != nil {
			status = RunFailed
		}
		endErr := r.publish(context.Background(), RunEvent{
			SessionID:     sessionID,
			RunID:         runID,
			Kind:          RunEnded,
			AfterEntrySeq: current.afterSeq(),
			Status:        status,
			Error:         errorText(err),
		})
		if endErr != nil {
			err = errors.Join(err, endErr)
		}
	}()

	message := messageFromInput(runID, input)
	entry, err := sess.Append(message)
	if err != nil {
		return err
	}
	current.setAfterEntrySeq(entry.Seq)
	if err = r.publish(runCtx, RunEvent{SessionID: sessionID, RunID: runID, Kind: Message, Entry: &entry}); err != nil {
		return err
	}
	history := sess.History()
	opened := current.openSteering(runCtx)
	if !opened {
		err = runCtx.Err()
		if err == nil {
			err = context.Canceled
		}
		return err
	}
	runStartedAttempted = true
	if err = r.publish(runCtx, RunEvent{SessionID: sessionID, RunID: runID, Kind: RunStarted, AfterEntrySeq: entry.Seq}); err != nil {
		return err
	}
	// 启动通知投递的协作输入进入初始历史，所有 Loop 的首次请求都能看到。
	initial, err := r.checkpoint(current, loops.CheckpointContinue)
	if err != nil {
		return err
	}
	history = append(history, initial...)

	invocation := loops.Invocation{
		SessionID:    sessionID,
		RunID:        runID,
		History:      history,
		SystemPrompt: prepared.SystemPrompt,
		LLMConfig: llm.RunConfig{
			Model:           runSettings.Model,
			ReasoningEffort: runSettings.ReasoningEffort,
		},
		ToolNames:   append([]string(nil), prepared.Tools...),
		Workspace:   runSettings.Workspace,
		InputSignal: current.inputSignalForWait,
		Emit: func(eventCtx context.Context, event loops.Event) error {
			return r.emit(eventCtx, sessionID, runID, sess, current, event)
		},
		Checkpoint: func(_ context.Context, phase loops.CheckpointPhase) ([]session.Message, error) {
			return r.checkpoint(current, phase)
		},
	}
	return loop.Run(runCtx, invocation)
}

func runResult(runID string, err error) RunResult {
	status := RunSucceeded
	if isCancellationOnly(err) {
		status = RunCancelled
	} else if err != nil {
		status = RunFailed
	}
	return RunResult{RunID: runID, Status: status, Err: err}
}

func isCancellationOnly(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !isCancellationOnly(cause) {
				return false
			}
		}
		return true
	}
	if unwrapped, ok := err.(interface{ Unwrap() error }); ok {
		cause := unwrapped.Unwrap()
		if cause != nil {
			return isCancellationOnly(cause)
		}
	}
	return errors.Is(err, context.Canceled)
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

func (r *Runner) release(sessionID string, current *liveRun) {
	current.closeSteering()
	r.mu.Lock()
	if r.live[sessionID] == current {
		delete(r.live, sessionID)
	}
	r.mu.Unlock()
}

func (r *Runner) emit(ctx context.Context, sessionID, runID string, sess *session.Session, current *liveRun, event loops.Event) error {
	if event.Kind == loops.EventMessage {
		if event.Message == nil {
			return fmt.Errorf("runner: message event has nil message")
		}
		message := *event.Message
		message.RunID = runID
		entry, err := sess.Append(message)
		if err != nil {
			return err
		}
		if message.Role == session.RoleAssistant {
			current.registerToolBlocks(message, event.StepSeq)
		}
		position := current.toolBlockForMessage(message, event.StepSeq, event.BlockSeq)
		return r.publish(ctx, RunEvent{
			SessionID:     sessionID,
			RunID:         runID,
			Kind:          Message,
			AfterEntrySeq: current.afterSeq(),
			StepSeq:       position.stepSeq,
			BlockSeq:      position.blockSeq,
			Entry:         &entry,
		})
	}
	runEvent, err := mapEvent(sessionID, runID, event)
	if err != nil {
		return err
	}
	runEvent.AfterEntrySeq = current.afterSeq()
	if event.Tool != nil {
		position := current.toolBlock(event.Tool.ID)
		runEvent.StepSeq = position.stepSeq
		runEvent.BlockSeq = position.blockSeq
	}
	return r.publish(ctx, runEvent)
}

func (r *Runner) publish(ctx context.Context, event RunEvent) error {
	return events.Publish(ctx, r.events, event)
}

func (r *Runner) checkpoint(current *liveRun, phase loops.CheckpointPhase) ([]session.Message, error) {
	messages, err := current.takeSteers(phase)
	if err != nil {
		return nil, err
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
		current.shutdown()
	}
	r.wg.Wait()
}

func mapEvent(sessionID, runID string, event loops.Event) (RunEvent, error) {
	out := RunEvent{SessionID: sessionID, RunID: runID, StepSeq: event.StepSeq, BlockSeq: event.BlockSeq}
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
	case loops.EventUsage:
		out.Kind = ContextUsage
		if event.Usage != nil {
			usage := Usage{
				InputTokens:     event.Usage.InputTokens,
				CacheReadTokens: event.Usage.CacheReadTokens,
				ContextWindow:   event.Usage.ContextWindow,
			}
			out.Usage = &usage
		}
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

func messageFromInput(runID string, input session.UserMessage) session.Message {
	return session.Message{RunID: runID, Role: session.RoleUser, Blocks: cloneBlocks(input.Blocks)}
}

func newRunID() (string, error) {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		return "", fmt.Errorf("runner: make run id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
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
