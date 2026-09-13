package subagents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Wait 等待任一指定任务的新终态 Run；不消费协作正文，也不启停 Run。
func (s *Subagents) Wait(ctx context.Context, parentSessionID string, input WaitInput) (WaitResponse, error) {
	err := ctx.Err()
	if err != nil {
		return WaitResponse{}, err
	}
	if parentSessionID == "" || len(input.TaskIDs) == 0 || input.Timeout < 0 || input.Timeout > 600*time.Second {
		return WaitResponse{}, fmt.Errorf("subagents: parent, task IDs and timeout between 0 and 600 seconds required")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return WaitResponse{}, ErrClosed
	}
	s.inFlight.Add(1)
	generation := s.families[parentSessionID].generation
	coords := make([]*taskCoord, 0, len(input.TaskIDs))
	seenTasks := make(map[string]bool)
	for _, id := range input.TaskIDs {
		if seenTasks[id] {
			continue
		}
		coords = append(coords, s.coords[id])
		seenTasks[id] = true
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
	targetSet := make(map[*taskCoord]bool)

	for {
		changed := s.changeSignal()
		response := WaitResponse{Tasks: []WaitResult{}, Notifications: []WaitResult{}}
		for _, coord := range coords {
			if coord == nil {
				return WaitResponse{}, ErrTaskNotFound
			}
			coord.mu.Lock()
			record := coord.record
			deliveryErr := coord.deliveryErr
			coord.mu.Unlock()
			if record.ParentSessionID != parentSessionID {
				return WaitResponse{}, ErrOwnershipMismatch
			}

			projection, projectErr := s.projectTask(record)
			if !targetSet[coord] {
				targetTurns[coord] = projection.view.Turn
				targetSet[coord] = true
			}
			response.Tasks = append(response.Tasks, makeWaitResult(projection.view))
			for _, notification := range projection.notifications {
				if notification.Turn < targetTurns[coord] || seen[notification.NotificationID] {
					continue
				}
				response.Notifications = append(response.Notifications, waitResultFromNotification(notification))
			}
			if projectErr != nil || deliveryErr != nil {
				return response, errors.Join(projectErr, deliveryErr)
			}
		}

		if ctx.Err() != nil {
			return response, ctx.Err()
		}
		if s.ctx.Err() != nil {
			return response, ErrClosed
		}
		s.mu.RLock()
		stopped := s.families[parentSessionID].generation != generation
		s.mu.RUnlock()
		if stopped {
			return response, ErrFamilyStopped
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

func makeWaitResult(task TaskView) WaitResult {
	return WaitResult{
		TaskID:        task.ID,
		Status:        task.Status,
		Turn:          task.Turn,
		RunID:         task.CurrentRunID,
		ResultEntryID: task.ResultEntryID,
		Error:         task.Error,
	}
}

func waitResultFromNotification(notification Notification) WaitResult {
	return WaitResult{
		NotificationID: notification.NotificationID,
		TaskID:         notification.TaskID,
		Status:         notification.Status,
		Turn:           notification.Turn,
		RunID:          notification.RunID,
		ResultEntryID:  notification.ResultEntryID,
		Error:          notification.Error,
	}
}

// List 列出属于指定父会话的关系，并从子 Session 即时投影状态与结果。
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
	var targetCoords []*taskCoord
	if taskID != "" {
		coord := s.coords[taskID]
		if coord == nil {
			s.mu.RUnlock()
			s.inFlight.Done()
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
	defer s.inFlight.Done()

	out := make([]TaskView, 0, len(targetCoords))
	var queryErrs []error
	for _, coord := range targetCoords {
		coord.mu.Lock()
		record := coord.record
		deliveryErr := coord.deliveryErr
		coord.mu.Unlock()
		if record.ParentSessionID != parentSessionID {
			if taskID != "" {
				return nil, ErrTaskNotFound
			}
			continue
		}
		projection, err := s.projectTask(record)
		out = append(out, projection.view)
		if err == nil {
			setup, settingsErr := s.settings.For(record.ChildSessionID)
			if settingsErr != nil {
				err = fmt.Errorf("%w: %v", ErrTaskNotUsable, settingsErr)
			} else {
				out[len(out)-1].AgentID = setup.AgentID
				out[len(out)-1].Model = setup.Model
				out[len(out)-1].ReasoningEffort = setup.ReasoningEffort
				out[len(out)-1].Workspace = setup.Workspace
			}
		}
		if err != nil || deliveryErr != nil {
			queryErrs = append(queryErrs, errors.Join(err, deliveryErr))
		}
	}
	if len(queryErrs) > 0 {
		return out, errors.Join(queryErrs...)
	}
	return out, nil
}
