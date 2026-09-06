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

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.inFlight.Add(1)
	coord := s.coords[taskID]
	s.mu.RUnlock()
	defer s.inFlight.Done()

	if coord == nil {
		return ErrTaskNotFound
	}

	coord.mu.Lock()
	if coord.task.ParentSessionID != parentSessionID {
		coord.mu.Unlock()
		return ErrOwnershipMismatch
	}
	childID := coord.task.ChildSessionID
	coord.mu.Unlock()

	return s.runner.Stop(childID)
}

// StopFamily 取消指定父会话已登记的孩子与父自身；并发派生的家族级屏障留待停止流程接入。
func (s *Subagents) StopFamily(ctx context.Context, parentSessionID string) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	if parentSessionID == "" {
		return fmt.Errorf("subagents: empty parent session id")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	s.inFlight.Add(1)
	taskIDs := append([]string(nil), s.parentTasks[parentSessionID]...)
	var targetCoords []*taskCoord
	for _, tid := range taskIDs {
		if coord := s.coords[tid]; coord != nil {
			targetCoords = append(targetCoords, coord)
		}
	}
	s.mu.RUnlock()
	defer s.inFlight.Done()

	for _, coord := range targetCoords {
		coord.mu.Lock()
		childID := coord.task.ChildSessionID
		status := coord.task.Status
		coord.mu.Unlock()
		if (status == StatusRunning || status == StatusPending) && childID != "" {
			_ = s.runner.Stop(childID)
		}
	}
	_ = s.runner.Stop(parentSessionID)
	return nil
}
