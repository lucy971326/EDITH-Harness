package subagents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Wait 等待任一指定任务的新完成通知；不消费协作正文，也不启动或停止 Run。
func (s *Subagents) Wait(ctx context.Context, parentSessionID string, input WaitInput) (WaitResponse, error) {
	err := ctx.Err()
	if err != nil {
		return WaitResponse{}, err
	}
	if parentSessionID == "" || len(input.TaskIDs) == 0 || input.Timeout < 0 || input.Timeout > 60*time.Second {
		return WaitResponse{}, fmt.Errorf("subagents: parent, task IDs and timeout between 0 and 60 seconds required")
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return WaitResponse{}, ErrClosed
	}
	s.inFlight.Add(1)
	coords := make([]*taskCoord, 0, len(input.TaskIDs))
	seenTasks := make(map[string]bool)
	for _, id := range input.TaskIDs {
		if !seenTasks[id] {
			coords = append(coords, s.coords[id])
			seenTasks[id] = true
		}
	}
	s.mu.RUnlock()
	defer s.inFlight.Done()
	seen := make(map[string]bool)
	for _, id := range input.SeenNotificationIDs {
		seen[id] = true
	}
	timer := time.NewTimer(input.Timeout)
	defer timer.Stop()
	wakeReason := ""
	targetTurns := make(map[*taskCoord]int)
	for {
		changed := s.changeSignal()
		response := WaitResponse{Tasks: []WaitResult{}, Notifications: []WaitResult{}}
		for _, coord := range coords {
			if coord == nil {
				return WaitResponse{}, ErrTaskNotFound
			}
			coord.mu.Lock()
			if coord.task.ParentSessionID != parentSessionID {
				coord.mu.Unlock()
				return WaitResponse{}, ErrOwnershipMismatch
			}
			if targetTurns[coord] == 0 {
				targetTurns[coord] = coord.task.Turn
			}
			res, pErr := makeWaitResult(coord.task, coord.task.Turn, coord.persistErr)
			response.Tasks = append(response.Tasks, res)
			for _, n := range coord.task.Notifications {
				if n.Turn >= targetTurns[coord] && !seen[n.NotificationID] {
					response.Notifications = append(response.Notifications, WaitResult{TaskID: n.TaskID, Status: n.Status, Turn: n.Turn, RunID: n.RunID, NotificationID: n.NotificationID, Error: n.Error})
				}
			}
			coord.mu.Unlock()
			if pErr != nil {
				return response, fmt.Errorf("%w: %v", ErrPersistFailed, pErr)
			}
		}
		if ctx.Err() != nil {
			return response, ctx.Err()
		}
		if s.ctx.Err() != nil {
			return response, ErrClosed
		}
		if len(response.Notifications) > 0 {
			response.Reason = "completed"
			err = s.deliver(parentSessionID)
			return response, err
		}
		if input.Timeout == 0 {
			response.Reason = "query"
			return response, nil
		}
		if wakeReason != "" {
			response.Reason = wakeReason
			return response, nil
		}
		select {
		case <-ctx.Done():
			return response, ctx.Err()
		case <-s.ctx.Done():
			return response, ErrClosed
		case <-timer.C:
			wakeReason = "timeout"
		case <-input.InputSignal:
			wakeReason = "input"
		case <-changed:
		}
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
func (s *Subagents) List(parentSessionID, taskID string) ([]TaskView, error) {
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

	out := make([]TaskView, 0, len(targetCoords))
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
		task := deepCopyTask(coord.task)
		if coord.persistErr != nil {
			persistErrs = append(persistErrs, fmt.Errorf("task %s persist error: %w", coord.task.ID, coord.persistErr))
		}
		if coord.deliveryErr != nil {
			persistErrs = append(persistErrs, coord.deliveryErr)
		}
		coord.mu.Unlock()
		results, err := s.readResults(task)
		out = append(out, TaskView{Task: task, Results: results})
		if err != nil {
			persistErrs = append(persistErrs, err)
		}
	}

	if len(persistErrs) > 0 {
		return out, errors.Join(persistErrs...)
	}
	return out, nil
}
