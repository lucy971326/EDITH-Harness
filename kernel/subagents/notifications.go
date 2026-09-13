package subagents

import (
	"errors"
	"fmt"
	"time"

	"harness/kernel/session"
)

func (s *Subagents) signalChange() {
	s.mu.Lock()
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

func (s *Subagents) changeSignal() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.changed
}

// deliverLoop 只向已有父 Run 重试通知，从不启动父会话。
func (s *Subagents) deliverLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	retryOnly := false
	for {
		changed := s.changeSignal()
		if retryOnly {
			_ = s.retryDeliveries()
		} else {
			_ = s.deliver("")
		}
		select {
		case <-s.ctx.Done():
			return
		case <-changed:
			retryOnly = false
		case <-ticker.C:
			retryOnly = true
		}
	}
}

func (s *Subagents) deliver(parentID string) error {
	return s.deliverTasks(parentID, false)
}

func (s *Subagents) retryDeliveries() error {
	return s.deliverTasks("", true)
}

func (s *Subagents) deliverTasks(parentID string, retryOnly bool) error {
	s.mu.RLock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.RUnlock()
		return nil
	}
	s.inFlight.Add(1)
	var coords []*taskCoord
	if parentID != "" {
		for _, id := range s.parentTasks[parentID] {
			coords = append(coords, s.coords[id])
		}
	} else {
		for _, coord := range s.coords {
			coords = append(coords, coord)
		}
	}
	s.mu.RUnlock()
	defer s.inFlight.Done()

	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	var errs []error
	for _, coord := range coords {
		coord.mu.Lock()
		record := coord.record
		coord.mu.Unlock()
		if parentID != "" && record.ParentSessionID != parentID {
			continue
		}
		if retryOnly {
			if _, waiting := s.pending[record.ID]; !waiting {
				continue
			}
		}

		s.mu.RLock()
		state, active := s.runner.State(record.ParentSessionID)
		stopped := active && s.families[record.ParentSessionID].stoppedRunID == state.RunID
		s.mu.RUnlock()
		if stopped {
			continue
		}

		projection, err := s.projectTask(record)
		if err != nil {
			if unavailableProjection(err) {
				delete(s.pending, record.ID)
				coord.mu.Lock()
				coord.deliveryErr = nil
				coord.mu.Unlock()
				continue
			}
			s.pending[record.ID] = struct{}{}
			coord.mu.Lock()
			coord.deliveryErr = err
			coord.mu.Unlock()
			errs = append(errs, err)
			continue
		}

		var deliveryErr error
		needsRetry := false
		for _, notification := range projection.notifications {
			if s.ctx.Err() != nil {
				needsRetry = true
				break
			}
			if _, confirmed := s.confirmed[notification.NotificationID]; confirmed {
				continue
			}
			message, err := notificationMessage(projection.view, notification)
			accepted := false
			if err == nil {
				accepted, err = s.runner.Receive(record.ParentSessionID, message)
			}
			if err != nil {
				deliveryErr = err
				needsRetry = true
				break
			}
			if accepted {
				s.confirmed[notification.NotificationID] = struct{}{}
				continue
			}
			needsRetry = true
		}
		if needsRetry {
			s.pending[record.ID] = struct{}{}
		} else {
			delete(s.pending, record.ID)
		}
		coord.mu.Lock()
		coord.deliveryErr = deliveryErr
		coord.mu.Unlock()
		if deliveryErr != nil {
			errs = append(errs, deliveryErr)
		}
	}
	return errors.Join(errs...)
}

func notificationMessage(task TaskView, notification Notification) (session.Message, error) {
	text := fmt.Sprintf("子任务 %s，第 %d 轮，状态：%s", notification.TaskID, notification.Turn, notification.Status)
	if notification.Error != "" {
		text += "\n错误：" + notification.Error
	}
	if notification.ResultEntryID != "" {
		found := false
		for _, result := range task.Results {
			if result.Turn != notification.Turn || result.RunID != notification.RunID || result.EntryID != notification.ResultEntryID {
				continue
			}
			text += "\n结果：" + result.Text
			found = true
			break
		}
		if !found {
			return session.Message{}, fmt.Errorf("subagents: task %s turn %d result missing from projection", task.ID, notification.Turn)
		}
	}
	return session.Message{
		Role:            session.RoleCollaboration,
		MessageID:       notification.NotificationID,
		SourceSessionID: notification.ChildSessionID,
		SourceRunID:     notification.RunID,
		Blocks:          []session.Block{{Kind: "text", Text: text}},
	}, nil
}
