package runner

import (
	"fmt"

	"harness/kernel/loops"
	"harness/kernel/session"
)

// Steer 把一条用户输入暂存到当前 Run 的下一个检查点。
func (r *Runner) Steer(sessionID string, input session.UserMessage) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}

	current.mu.Lock()
	defer current.mu.Unlock()
	if current.steeringClosed {
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	current.steers = append(current.steers, session.UserMessage{Blocks: cloneBlocks(input.Blocks)})
	return nil
}

// FollowUp 把一条用户输入排入当前 Run 之后的下一轮独立执行。
func (r *Runner) FollowUp(sessionID string, input session.UserMessage) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.followUpsClosed {
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	current.followUps = append(current.followUps, session.UserMessage{Blocks: cloneBlocks(input.Blocks)})
	return nil
}

// Stop 取消一本 Session 当前尚未结束的 Run，不清空 FollowUp。
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

func (r *liveRun) takeSteers(phase loops.CheckpointPhase) ([]session.UserMessage, error) {
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

func (r *liveRun) takeFollowUp() (session.UserMessage, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.followUps) == 0 {
		r.followUpsClosed = true
		return session.UserMessage{}, false
	}
	next := r.followUps[0]
	r.followUps = r.followUps[1:]
	return next, true
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

func (r *liveRun) openSteering() {
	r.mu.Lock()
	r.stopped = false
	r.steeringClosed = false
	r.mu.Unlock()
}

func (r *liveRun) setAfterEntrySeq(seq uint64) {
	r.mu.Lock()
	r.afterEntrySeq = seq
	r.mu.Unlock()
}

func (r *liveRun) setRunID(id string) {
	r.mu.Lock()
	r.runID = id
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
