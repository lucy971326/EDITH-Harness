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

// deliverLoop 只向已有 Run 重试通知，从不启动父会话。
func (s *Subagents) deliverLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		changed := s.changeSignal()
		// 先获取广播代次再读取状态，不漏掉读取期间发生的变化。
		// 错误留在任务查询中；下一轮启动也会同步返回投递错误。
		_ = s.deliver("")
		select {
		case <-s.ctx.Done():
			return
		case <-changed:
		case <-ticker.C:
		}
	}
}

func (s *Subagents) deliver(parentID string) error {
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
		task := deepCopyTask(coord.task)
		pErr := coord.persistErr
		coord.mu.Unlock()
		if parentID != "" && task.ParentSessionID != parentID {
			continue
		}
		if pErr != nil {
			errs = append(errs, pErr)
			continue // 未可靠保存的结果不得投递。
		}
		for i, notification := range task.Notifications {
			if notification.Delivered || s.ctx.Err() != nil {
				continue
			}
			message, err := s.notificationMessage(task, notification)
			accepted := false
			if err == nil {
				accepted, err = s.runner.Receive(task.ParentSessionID, message)
			}
			coord.mu.Lock()
			if accepted {
				// 确认写盘失败保持未确认，下次以相同 ID 重试。
				coord.task.Notifications[i].Delivered = true
				saveErr := s.store.saveTask(coord.task)
				if saveErr != nil {
					coord.task.Notifications[i].Delivered = false
					err = errors.Join(err, saveErr)
				}
			}
			if err != nil || accepted {
				coord.deliveryErr = err
			}
			coord.mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				break
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Subagents) notificationMessage(task Task, n Notification) (session.Message, error) {
	text := fmt.Sprintf("子任务 %s，第 %d 轮，状态：%s", n.TaskID, n.Turn, n.Status)
	if n.Error != "" {
		text += "\n错误：" + n.Error
	}
	if n.ResultEntryID != "" {
		results, err := s.readResults(Task{ID: task.ID, ChildSessionID: task.ChildSessionID, Turns: []TurnRecord{{Turn: n.Turn, RunID: n.RunID, ResultEntryID: n.ResultEntryID}}})
		if err != nil {
			return session.Message{}, err
		}
		text += "\n结果：" + results[0].Text
	}
	return session.Message{Role: session.RoleCollaboration, MessageID: n.NotificationID, SourceSessionID: n.ChildSessionID, SourceRunID: n.RunID, Blocks: []session.Block{{Kind: "text", Text: text}}}, nil
}
