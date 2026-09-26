package subagents

import (
	"fmt"
	"sort"
)

// ReserveDeletion 在关系锁下冻结根及后代，把待删清单先交给调用方持久保存。
// 忙碌的运行或正在写入的委派会拒绝删除；调用方不需要等待执行完成。
func (s *Subagents) ReserveDeletion(roots []string, persist func(sessionIDs, taskIDs []string) error) ([]string, []string, error) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return nil, nil, ErrClosed
	}

	seen := make(map[string]struct{})
	queue := append([]string(nil), roots...)
	tasks := make([]string, 0)
	coords := make([]*taskCoord, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, exists := seen[id]; exists {
			continue
		}
		if _, deleted := s.deletedSessions[id]; deleted {
			return nil, nil, fmt.Errorf("%w: session deletion in progress", ErrTaskActive)
		}
		seen[id] = struct{}{}
		if _, active := s.runner.State(id); active {
			return nil, nil, fmt.Errorf("%w: session %s is running", ErrTaskActive, id)
		}
		for _, taskID := range s.parentTasks[id] {
			coord := s.coords[taskID]
			if coord == nil {
				return nil, nil, fmt.Errorf("%w: missing task %s", ErrInvalidTaskData, taskID)
			}
			tasks = append(tasks, taskID)
			coords = append(coords, coord)
			queue = append(queue, coord.record.ChildSessionID)
		}
	}
	locked := make([]*taskCoord, 0, len(coords))
	defer func() {
		for _, coord := range locked {
			coord.mu.Unlock()
		}
	}()
	for _, coord := range coords {
		if !coord.mu.TryLock() {
			return nil, nil, fmt.Errorf("%w: task operation in progress", ErrTaskActive)
		}
		locked = append(locked, coord)
	}
	sessionIDs := make([]string, 0, len(seen))
	for id := range seen {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)
	sort.Strings(tasks)
	if err := persist(sessionIDs, tasks); err != nil {
		return nil, nil, err
	}
	for _, id := range sessionIDs {
		s.deletedSessions[id] = struct{}{}
		delete(s.families, id)
	}
	for _, coord := range coords {
		s.forgetTaskLocked(coord.record)
		delete(s.pending, coord.record.ID)
		delete(s.confirmed, coord.record.ID)
	}
	return sessionIDs, tasks, nil
}
