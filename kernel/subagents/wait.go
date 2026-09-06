package subagents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Wait 等待服务层持久化完成/失败信号，针对指定轮次返回结果，防并发新轮干扰。
func (s *Subagents) Wait(ctx context.Context, parentSessionID, taskID string, timeout time.Duration) (WaitResult, error) {
	err := ctx.Err()
	if err != nil {
		return WaitResult{}, err
	}
	if taskID == "" {
		return WaitResult{}, fmt.Errorf("subagents: empty task id")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return WaitResult{}, ErrClosed
	}
	s.inFlight.Add(1)
	coord := s.coords[taskID]
	s.mu.RUnlock()
	defer s.inFlight.Done()

	if coord == nil {
		return WaitResult{}, ErrTaskNotFound
	}

	coord.mu.Lock()
	if coord.task.ParentSessionID != parentSessionID {
		coord.mu.Unlock()
		return WaitResult{}, ErrOwnershipMismatch
	}

	targetTurn := coord.task.Turn
	// 若已经不是运行中或超时时间 <= 0，立即返回当前目标轮次的状态
	if (coord.task.Status != StatusRunning && coord.task.Status != StatusPending) || timeout <= 0 {
		res, pErr := makeWaitResult(coord.task, targetTurn, coord.persistErr)
		coord.mu.Unlock()
		if pErr != nil {
			return WaitResult{}, fmt.Errorf("%w: %v", ErrPersistFailed, pErr)
		}
		return res, nil
	}

	waitCh := coord.waitCh
	coord.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return WaitResult{}, ctx.Err()
	case <-s.ctx.Done():
		return WaitResult{}, ErrClosed
	case <-timer.C:
		coord.mu.Lock()
		res, pErr := makeWaitResult(coord.task, targetTurn, coord.persistErr)
		coord.mu.Unlock()
		if pErr != nil {
			return WaitResult{}, fmt.Errorf("%w: %v", ErrPersistFailed, pErr)
		}
		return res, nil
	case <-waitCh:
		coord.mu.Lock()
		res, pErr := makeWaitResult(coord.task, targetTurn, coord.persistErr)
		coord.mu.Unlock()
		if pErr != nil {
			return WaitResult{}, fmt.Errorf("%w: %v", ErrPersistFailed, pErr)
		}
		return res, nil
	}
}

func makeWaitResult(task Task, targetTurn int, persistErr error) (WaitResult, error) {
	for i := len(task.Turns) - 1; i >= 0; i-- {
		if task.Turns[i].Turn == targetTurn {
			rec := task.Turns[i]
			return WaitResult{
				TaskID:        task.ID,
				Status:        rec.Status,
				Turn:          rec.Turn,
				RunID:         rec.RunID,
				ResultEntryID: rec.ResultEntryID,
				Error:         rec.Error,
			}, persistErr
		}
	}
	return WaitResult{
		TaskID:        task.ID,
		Status:        task.Status,
		Turn:          task.Turn,
		RunID:         task.CurrentRunID,
		ResultEntryID: task.ResultEntryID,
		Error:         task.Error,
	}, persistErr
}

// List 列出属于指定父会话的任务记录。严格统一锁序，不在持 s.mu 时等待 coord.mu。
func (s *Subagents) List(parentSessionID, taskID string) ([]Task, error) {
	if parentSessionID == "" {
		return nil, fmt.Errorf("subagents: empty parent session id")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrClosed
	}
	s.inFlight.Add(1)
	defer s.inFlight.Done()

	var targetCoords []*taskCoord
	if taskID != "" {
		coord := s.coords[taskID]
		if coord == nil {
			s.mu.RUnlock()
			return nil, ErrTaskNotFound
		}
		targetCoords = []*taskCoord{coord}
	} else {
		ids := s.parentTasks[parentSessionID]
		targetCoords = make([]*taskCoord, 0, len(ids))
		for _, id := range ids {
			if coord := s.coords[id]; coord != nil {
				targetCoords = append(targetCoords, coord)
			}
		}
	}
	s.mu.RUnlock()

	out := make([]Task, 0, len(targetCoords))
	var persistErrs []error
	for _, coord := range targetCoords {
		coord.mu.Lock()
		if coord.task.ParentSessionID != parentSessionID {
			coord.mu.Unlock()
			if taskID != "" {
				return nil, ErrTaskNotFound
			}
			continue
		}
		out = append(out, deepCopyTask(coord.task))
		if coord.persistErr != nil {
			persistErrs = append(persistErrs, fmt.Errorf("task %s persist error: %w", coord.task.ID, coord.persistErr))
		}
		coord.mu.Unlock()
	}

	if len(persistErrs) > 0 {
		return out, errors.Join(persistErrs...)
	}
	return out, nil
}
