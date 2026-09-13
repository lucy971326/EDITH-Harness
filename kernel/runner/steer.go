package runner

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// ErrRunChanged 表示预期运行已结束、关闭插话或被另一轮替代。
var ErrRunChanged = errors.New("runner: expected run is no longer accepting input")

// Steer 等待一条用户输入在当前 Run 的下一个安全检查点落账。
func (r *Runner) Steer(sessionID string, input session.UserMessage) error {
	return r.steer(context.Background(), sessionID, "", input)
}

// SteerContext 只让调用方放弃等待响应；已经准入的输入仍由当前 Run 在检查点决定结果。
func (r *Runner) SteerContext(ctx context.Context, sessionID string, input session.UserMessage) error {
	return r.steer(ctx, sessionID, "", input)
}

// SteerRun 只向预期的那轮插话；身份检查与准入、落账不可分割。
func (r *Runner) SteerRun(sessionID, expectedRunID string, input session.UserMessage) error {
	if expectedRunID == "" {
		return ErrRunChanged
	}
	return r.steer(context.Background(), sessionID, expectedRunID, input)
}

// SteerRunContext 是带预期 Run 身份的 SteerContext。
func (r *Runner) SteerRunContext(ctx context.Context, sessionID, expectedRunID string, input session.UserMessage) error {
	if expectedRunID == "" {
		return ErrRunChanged
	}
	return r.steer(ctx, sessionID, expectedRunID, input)
}

func (r *Runner) steer(ctx context.Context, sessionID, expectedRunID string, input session.UserMessage) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	current, err := r.current(sessionID)
	if err != nil {
		if expectedRunID != "" {
			return ErrRunChanged
		}
		return err
	}

	_, err = r.sessions.Get(sessionID)
	if err != nil {
		return err
	}

	// 准入和交给检查点不可分割；消息尚未完成工具批次时不能提前落账。
	result := make(chan error, 1)
	current.handoff.Lock()
	current.mu.Lock()
	if current.steeringState != steeringOpen || (expectedRunID != "" && current.runID != expectedRunID) {
		current.mu.Unlock()
		current.handoff.Unlock()
		if expectedRunID != "" {
			return ErrRunChanged
		}
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	runID := current.runID
	message := messageFromInput(runID, input)
	current.pendingInputs = append(current.pendingInputs, pendingInput{message: message, result: result})
	current.signalInputLocked()
	current.mu.Unlock()
	current.handoff.Unlock()
	select {
	case err = <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop 取消一本 Session 当前尚未结束的 Run。
func (r *Runner) Stop(sessionID string) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}
	current.stop()
	return nil
}

// StopRun 仅取消指定身份的运行；已经结束或换轮时返回 false，不误停新一轮。
func (r *Runner) StopRun(sessionID, runID string) bool {
	current, err := r.current(sessionID)
	if err != nil || current.runID != runID {
		return false
	}
	current.stop()
	return true
}

func (r *Runner) current(sessionID string) (*liveRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.live[sessionID]
	if !ok {
		return nil, fmt.Errorf("runner: session %q is not running", sessionID)
	}
	return current, nil
}

// State 返回一本 Session 正在运行的稳定身份；空闲时返回 false。
func (r *Runner) State(sessionID string) (RunState, bool) {
	current, err := r.current(sessionID)
	if err != nil {
		return RunState{}, false
	}
	return current.state()
}

// RunSettings 返回指定活跃 Run 启动时保存的配置快照。
func (r *Runner) RunSettings(sessionID, runID string) (settings.SessionSettings, error) {
	current, err := r.current(sessionID)
	if err != nil {
		return settings.SessionSettings{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.runID != runID {
		return settings.SessionSettings{}, fmt.Errorf("runner: run %q is not active for session %q", runID, sessionID)
	}
	if current.settings == nil {
		return settings.SessionSettings{}, fmt.Errorf("runner: run %q is still preparing", runID)
	}
	return *current.settings, nil
}

func (r *Runner) checkpoint(sess *session.Session, sessionID string, current *liveRun, phase loops.CheckpointPhase) ([]session.Message, error) {
	if phase != loops.CheckpointContinue && phase != loops.CheckpointFinal {
		return nil, fmt.Errorf("runner: invalid checkpoint phase %d", phase)
	}

	type committedInput struct {
		pending pendingInput
		entry   session.Entry
		seq     uint64
		after   uint64
	}

	// Checkpoint 与输入准入共用 handoff，最终边界不能从正在接收的输入中间穿过。
	current.handoff.Lock()
	current.mu.Lock()
	pending := current.pendingInputs
	current.pendingInputs = nil
	if len(pending) > 0 {
		current.inputSignal = make(chan struct{})
	} else if phase == loops.CheckpointFinal {
		current.steeringState = steeringClosed
	}
	current.mu.Unlock()
	if len(pending) == 0 {
		current.handoff.Unlock()
		return nil, nil
	}

	committed := make([]committedInput, 0, len(pending))
	var appendErr error
	for _, input := range pending {
		entry, err := sess.Append(input.message)
		if err != nil {
			appendErr = err
			break
		}
		current.mu.Lock()
		current.persisted[entry.ID] = struct{}{}
		current.outputAfterSeq = entry.Seq
		current.updateSeq++
		seq := current.updateSeq
		after := current.afterEntrySeq
		current.mu.Unlock()
		committed = append(committed, committedInput{pending: input, entry: entry, seq: seq, after: after})
	}
	current.handoff.Unlock()

	messages := make([]session.Message, 0, len(committed))
	var firstErr error
	for _, item := range committed {
		entry := item.entry
		err := r.publish(context.Background(), RunEvent{
			SessionID:     sessionID,
			RunID:         current.runID,
			Kind:          Message,
			EntryID:       entry.ID,
			AfterEntrySeq: item.after,
			Entry:         &entry,
			UpdateSeq:     item.seq,
			SeqEpoch:      r.epoch,
		})
		item.pending.complete(err)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		messages = append(messages, item.pending.message)
	}

	if appendErr != nil {
		if firstErr == nil {
			firstErr = appendErr
		}
		for _, input := range pending[len(committed):] {
			input.complete(appendErr)
		}
	}
	if firstErr != nil {
		current.stop()
		return nil, firstErr
	}
	return messages, nil
}

func (p pendingInput) complete(err error) {
	if p.result != nil {
		p.result <- err
	}
}

func (r *liveRun) openSteering(ctx context.Context) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.steeringState != steeringInitializing {
		return false
	}
	err := ctx.Err()
	if err != nil {
		r.steeringState = steeringClosed
		return false
	}
	r.steeringState = steeringOpen
	return true
}

func (r *liveRun) inputSignalForWait() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inputSignal
}

func (r *liveRun) setAfterEntrySeq(seq uint64) {
	r.mu.Lock()
	r.afterEntrySeq = seq
	r.outputAfterSeq = seq
	r.mu.Unlock()
}

func (r *liveRun) state() (RunState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := RunRunning
	if r.ended {
		status = r.endStatus
		if status == "" {
			status = RunSucceeded
		}
	}
	return RunState{RunID: r.runID, AfterEntrySeq: r.afterEntrySeq, Status: status}, !r.ended
}

func (r *liveRun) afterSeq() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.afterEntrySeq
}

func (r *liveRun) registerToolCallsLocked(entryID string, message session.Message) {
	for index, block := range message.Blocks {
		if block.Kind != "tool-call" || block.Tool == nil {
			continue
		}
		r.toolCalls[block.Tool.ID] = toolCallLoc{entryID: entryID, blockSeq: uint64(index + 1)}
	}
}

func (r *liveRun) toolCall(id string) toolCallLoc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolCalls[id]
}

func (r *liveRun) signalInputLocked() {
	if r.inputSignal == nil {
		r.inputSignal = make(chan struct{})
		return
	}
	select {
	case <-r.inputSignal:
	default:
		close(r.inputSignal)
	}
}

func (r *liveRun) stop() {
	r.finishInputs()
}

func (r *liveRun) shutdown() {
	r.finishInputs()
}

func (r *liveRun) finishInputs() {
	r.handoff.Lock()
	r.mu.Lock()
	r.steeringState = steeringClosed
	pending := r.pendingInputs
	r.pendingInputs = nil
	cancel := r.cancel
	if cancel != nil {
		cancel()
	}
	r.mu.Unlock()
	r.handoff.Unlock()
	for _, input := range pending {
		input.complete(ErrRunChanged)
	}
}
