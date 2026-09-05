package runner

import (
	"context"
	"fmt"

	"harness/kernel/loops"
	"harness/kernel/session"
)

// Steer 先把一条用户输入落账，再交给当前 Run 的下一个检查点。
func (r *Runner) Steer(sessionID string, input session.UserMessage) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}

	current.mu.Lock()
	if current.steeringClosed || current.runID == "" {
		current.mu.Unlock()
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	message := messageFromInput(current.runID, input)
	entry, err := r.sessions.Get(sessionID)
	if err != nil {
		current.mu.Unlock()
		return err
	}
	durable, err := entry.Append(message)
	if err != nil {
		current.mu.Unlock()
		return err
	}
	current.steers = append(current.steers, message)
	afterEntrySeq := current.afterEntrySeq
	current.mu.Unlock()

	err = r.publish(context.Background(), RunEvent{
		SessionID:     sessionID,
		RunID:         message.RunID,
		Kind:          Message,
		AfterEntrySeq: afterEntrySeq,
		Entry:         &durable,
	})
	if err != nil {
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
	return current.state(), true
}

func (r *liveRun) takeSteers(phase loops.CheckpointPhase) ([]session.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if phase != loops.CheckpointContinue && phase != loops.CheckpointFinal {
		return nil, fmt.Errorf("runner: invalid checkpoint phase %d", phase)
	}
	steers := r.steers
	r.steers = nil
	if phase == loops.CheckpointFinal && len(steers) == 0 {
		r.steeringClosed = true
	}
	return steers, nil
}

func (r *liveRun) setCancel(cancel func()) {
	r.mu.Lock()
	r.cancel = cancel
	stopped := r.stopped
	r.mu.Unlock()
	if stopped {
		cancel()
	}
}

func (r *liveRun) openSteering(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return false
	}
	r.stopped = false
	r.steeringClosed = false
	r.runID = runID
	return true
}

func (r *liveRun) startExclusive(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return false
	}
	r.stopped = false
	r.steeringClosed = true
	r.runID = runID
	return true
}

func (r *liveRun) setAfterEntrySeq(seq uint64) {
	r.mu.Lock()
	r.afterEntrySeq = seq
	r.mu.Unlock()
}

func (r *liveRun) state() RunState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RunState{RunID: r.runID, AfterEntrySeq: r.afterEntrySeq}
}

func (r *liveRun) afterSeq() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.afterEntrySeq
}

func (r *liveRun) registerToolBlocks(message session.Message, stepSeq uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, block := range message.Blocks {
		if block.Kind != "tool-call" || block.Tool == nil {
			continue
		}
		r.toolBlocks[block.Tool.ID] = toolBlock{stepSeq: stepSeq, blockSeq: uint64(index + 1)}
	}
}

func (r *liveRun) toolBlock(id string) toolBlock {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolBlocks[id]
}

func (r *liveRun) toolBlockForMessage(message session.Message, stepSeq, blockSeq uint64) toolBlock {
	if message.Role != session.RoleTool || len(message.Blocks) != 1 || message.Blocks[0].Result == nil {
		return toolBlock{stepSeq: stepSeq, blockSeq: blockSeq}
	}
	return r.toolBlock(message.Blocks[0].Result.ID)
}

func (r *liveRun) stop() {
	r.mu.Lock()
	r.stopped = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *liveRun) shutdown() {
	r.mu.Lock()
	r.closing = true
	r.stopped = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
