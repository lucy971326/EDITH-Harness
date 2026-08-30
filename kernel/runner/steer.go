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
	if current.closed {
		return fmt.Errorf("runner: session %q is not running", sessionID)
	}
	current.steers = append(current.steers, session.UserMessage{Blocks: cloneBlocks(input.Blocks)})
	return nil
}

// Stop 取消一本 Session 当前尚未结束的 Run。
func (r *Runner) Stop(sessionID string) error {
	current, err := r.current(sessionID)
	if err != nil {
		return err
	}
	current.cancel()
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
		r.closed = true
	}
	return steers, nil
}
