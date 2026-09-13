package subagents

import (
	"fmt"

	"harness/kernel/session"
)

// startTurn 在持有 coord.mu 时启动子 Run；运行状态只由 Runner 保存。
func (s *Subagents) startTurn(coord *taskCoord, input session.UserMessage) (string, error) {
	if s.ctx.Err() != nil {
		return "", ErrClosed
	}
	err := s.childStartError(coord)
	if err != nil {
		return "", err
	}
	handle, err := s.runner.Start(s.ctx, coord.record.ChildSessionID, input)
	if err != nil {
		return "", fmt.Errorf("start child run: %w", err)
	}
	s.trackRun(handle)

	stopErr := s.childStartError(coord)
	if stopErr != nil {
		s.runner.StopRun(coord.record.ChildSessionID, handle.RunID())
		return "", stopErr
	}
	if s.ctx.Err() != nil {
		return "", ErrClosed
	}
	return handle.RunID(), nil
}

func notificationID(taskID string, turn int) string {
	return fmt.Sprintf("%s-turn-%d", taskID, turn)
}
