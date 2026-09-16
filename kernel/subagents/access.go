package subagents

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/runner"
)

// Snapshot 返回一个属于指定父会话的子任务及其账本运行投影。
func (s *Subagents) Snapshot(parentSessionID, taskID string) (TaskSnapshot, error) {
	tasks, err := s.List(parentSessionID, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if len(tasks) != 1 {
		return TaskSnapshot{}, ErrTaskNotFound
	}
	view, err := s.runner.SessionView(tasks[0].ChildSessionID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if view.Runs == nil {
		view.Runs = []runner.RunState{}
	}
	return TaskSnapshot{Task: tasks[0], View: view}, nil
}

// UpdateSettings 在孩子空闲时只更新模型与思考档位。
func (s *Subagents) UpdateSettings(ctx context.Context, parentSessionID, taskID string, input TaskSettingsInput) (TaskSettingsResult, error) {
	err := ctx.Err()
	if err != nil {
		return TaskSettingsResult{}, err
	}
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.ReasoningEffort) == "" {
		return TaskSettingsResult{}, ErrInvalidSettings
	}

	s.mu.RLock()
	if s.ctx.Err() != nil {
		s.mu.RUnlock()
		return TaskSettingsResult{}, ErrClosed
	}
	s.work.Add(1)
	coord := s.coords[taskID]
	s.mu.RUnlock()
	defer s.work.Done()
	if coord == nil {
		return TaskSettingsResult{}, ErrTaskNotFound
	}

	coord.mu.Lock()
	defer coord.mu.Unlock()
	if coord.record.ParentSessionID != parentSessionID {
		return TaskSettingsResult{}, ErrOwnershipMismatch
	}
	if _, active := s.runner.State(coord.record.ChildSessionID); active {
		return TaskSettingsResult{}, ErrTaskActive
	}

	valid := false
	for _, model := range s.models.Models() {
		if model.ID != input.Model {
			continue
		}
		for _, effort := range model.ReasoningEfforts {
			if effort == input.ReasoningEffort {
				valid = true
				break
			}
		}
		break
	}
	if !valid {
		return TaskSettingsResult{}, ErrInvalidSettings
	}

	next, err := s.settings.For(coord.record.ChildSessionID)
	if err != nil {
		return TaskSettingsResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}
	next.Model = input.Model
	next.ReasoningEffort = input.ReasoningEffort
	err = s.settings.Put(coord.record.ChildSessionID, next)
	if err != nil {
		return TaskSettingsResult{}, err
	}
	return next, nil
}

// ReadRunDiffFile 读取属于该子任务的一份 Run Diff。
func (s *Subagents) ReadRunDiffFile(parentSessionID, taskID, runID, path string) (runner.RunDiffFile, error) {
	snapshot, err := s.Snapshot(parentSessionID, taskID)
	if err != nil {
		return runner.RunDiffFile{}, err
	}
	return s.runner.ReadRunDiffFile(snapshot.Task.ChildSessionID, runID, path)
}

// RevertRunDiffFile 在归属校验后撤销孩子一轮中的一个文件。
func (s *Subagents) RevertRunDiffFile(parentSessionID, taskID, runID, path string, expectedRevision uint64) (runner.RunDiffSummary, error) {
	snapshot, err := s.Snapshot(parentSessionID, taskID)
	if err != nil {
		return runner.RunDiffSummary{}, err
	}
	return s.runner.RevertRunDiffFile(snapshot.Task.ChildSessionID, runID, path, expectedRevision)
}
