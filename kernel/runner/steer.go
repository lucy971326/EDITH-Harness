package runner

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// Steer 先把一条用户输入落账，再交给当前 Run 的下一个检查点。
func (r *Runner) Steer(sessionID string, input session.UserMessage) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}

	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return err
	}

	// 准入、落账和交给检查点是一件事；最终检查点不能从中间穿过。
	current.handoff.Lock()
	current.mu.Lock()
	if current.steeringState != steeringOpen {
		current.mu.Unlock()
		current.handoff.Unlock()
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	runID := current.runID
	message := messageFromInput(runID, input)
	durable, err := sess.Append(message)
	if err != nil {
		current.mu.Unlock()
		current.handoff.Unlock()
		return err
	}

	current.steers = append(current.steers, message)
	current.pendingAfterSeq = durable.Seq
	current.signalInputLocked()
	current.persisted[durable.ID] = struct{}{}
	current.updateSeq++
	seq := current.updateSeq
	afterEntrySeq := current.afterEntrySeq
	current.inputPublications.Add(1)
	current.mu.Unlock()
	current.handoff.Unlock()
	defer current.inputPublications.Done()

	err = r.publish(context.Background(), RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          Message,
		EntryID:       durable.ID,
		AfterEntrySeq: afterEntrySeq,
		Entry:         &durable,
		UpdateSeq:     seq,
		SeqEpoch:      r.epoch,
	})
	if err != nil {
		current.mu.Lock()
		// 保留 Steer 的既有语义：发布失败取消本轮，原错误返回给调用方。
		current.inputErr = errors.Join(current.inputErr, context.Canceled)
		current.mu.Unlock()
		current.stop()
		return err
	}
	return nil
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

func (r *liveRun) takeSteers(phase loops.CheckpointPhase) ([]session.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if phase != loops.CheckpointContinue && phase != loops.CheckpointFinal {
		return nil, fmt.Errorf("runner: invalid checkpoint phase %d", phase)
	}
	steers := r.steers
	r.steers = nil
	if len(steers) > 0 {
		r.inputSignal = make(chan struct{})
		r.outputAfterSeq = r.pendingAfterSeq
	}
	if phase == loops.CheckpointFinal && len(steers) == 0 {
		r.steeringState = steeringClosed
	}
	return steers, nil
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
	r.closeSteering()
}

func (r *liveRun) shutdown() {
	r.closeSteering()
}

func (r *liveRun) closeSteering() {
	r.mu.Lock()
	r.steeringState = steeringClosed
	cancel := r.cancel
	if cancel != nil {
		cancel()
	}
	r.mu.Unlock()
}
