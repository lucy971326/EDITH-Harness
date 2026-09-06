package subagents

import (
	"context"
	"fmt"
)

// Stop 请求取消指定孩子的当前运行，不立即伪造 Cancelled 状态。
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
	if s.closed {
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
	// 子 Session 身份保存在关系索引，不等待 coord.mu 中的启动或写盘操作。
	for childID, id := range s.childSessions {
		if id == taskID {
			s.stopRunLocked(childID)
			break
		}
	}
	return nil
}

// StopFamily 先使旧操作失效，再取消父和已登记的孩子；不等待孩子的启动磁盘锁。
func (s *Subagents) StopFamily(ctx context.Context, parentSessionID string) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	if parentSessionID == "" {
		return fmt.Errorf("subagents: empty parent session id")
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	family := s.families[parentSessionID]
	family.generation++
	if state, active := s.runner.State(parentSessionID); active {
		family.stoppedRunID = state.RunID
		s.runner.StopRun(parentSessionID, state.RunID)
	}
	s.families[parentSessionID] = family
	for childID, taskID := range s.childSessions {
		coord := s.coords[taskID]
		if coord.admission.parentSessionID == parentSessionID {
			coord.stopRequested = true
			coord.stopGeneration++
			s.stopRunLocked(childID)
		}
	}
	s.mu.Unlock()
	s.signalChange()
	return nil
}

// stopRunLocked 的调用方持有 s.mu；只取消观察到的 Run，已结束不算错误。
func (s *Subagents) stopRunLocked(sessionID string) {
	if state, active := s.runner.State(sessionID); active {
		s.runner.StopRun(sessionID, state.RunID)
	}
}
