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

func (r *liveRun) stop() {
	r.mu.Lock()
	r.stopped = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
