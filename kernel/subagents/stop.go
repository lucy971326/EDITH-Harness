package subagents

import (
	"context"
	"fmt"
)

// Stop 请求取消指定孩子及其全部后代的当前运行，不立即伪造 Cancelled 状态。
func (s *Subagents) Stop(ctx context.Context, parentSessionID, taskID string) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	if taskID == "" {
		return fmt.Errorf("subagents: empty task id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	coord := s.coords[taskID]

	if coord == nil {
		return ErrTaskNotFound
	}

	if coord.admission.parentSessionID != parentSessionID {
		return ErrOwnershipMismatch
	}
	coord.stopRequested = true
	coord.stopGeneration++
	s.stopSessionTreeLocked(coord.record.ChildSessionID)
	return nil
}

// StopFamily 先使旧操作失效，再取消父和全部后代；不等待孩子的启动磁盘锁。
func (s *Subagents) StopFamily(ctx context.Context, parentSessionID string) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	if parentSessionID == "" {
		return fmt.Errorf("subagents: empty parent session id")
	}

	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return ErrClosed
	}
	s.stopSessionTreeLocked(parentSessionID)
	s.mu.Unlock()
	s.signalChange()
	return nil
}

// stopSessionTreeLocked 取消一个 Session 及其全部后代，并使各层已接收的旧操作失效。
// 调用方持有 s.mu；关系在持锁期间不会新增。
func (s *Subagents) stopSessionTreeLocked(rootSessionID string) {
	queue := []string{rootSessionID}
	seen := make(map[string]struct{})
	for len(queue) > 0 {
		sessionID := queue[0]
		queue = queue[1:]
		if _, duplicate := seen[sessionID]; duplicate {
			continue
		}
		seen[sessionID] = struct{}{}

		family := s.families[sessionID]
		family.generation++
		if state, active := s.runner.State(sessionID); active {
			family.stoppedRunID = state.RunID
			s.runner.StopRun(sessionID, state.RunID)
		}
		s.families[sessionID] = family

		for _, taskID := range s.parentTasks[sessionID] {
			coord := s.coords[taskID]
			if coord == nil {
				continue
			}
			coord.stopRequested = true
			coord.stopGeneration++
			queue = append(queue, coord.record.ChildSessionID)
		}
	}
}
