package subagents

import (
	"errors"
	"fmt"
	"strings"

	"harness/kernel/runner"
	"harness/kernel/session"
)

// 数据。一条关系的查询投影与可投递终态通知。
type taskProjection struct {
	view          TaskView
	notifications []Notification
}

func (s *Subagents) projectTask(record TaskRecord) (taskProjection, error) {
	projection := taskProjection{view: TaskView{
		ID:              record.ID,
		ParentSessionID: record.ParentSessionID,
		ChildSessionID:  record.ChildSessionID,
		Description:     record.Description,
		Turns:           []TurnRecord{},
		Results:         []TurnResult{},
	}}

	_, err := s.sessions.Get(record.ChildSessionID)
	if err != nil {
		projection.view.Status = StatusFailed
		projection.view.Error = fmt.Sprintf("%v: %v", ErrTaskNotUsable, err)
		return projection, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}
	childView, err := s.runner.SessionView(record.ChildSessionID)
	if err != nil {
		projection.view.Status = StatusFailed
		projection.view.Error = err.Error()
		return projection, err
	}
	runs := append([]runner.RunState(nil), childView.Runs...)
	if live, active := s.runner.State(record.ChildSessionID); active && !containsRun(runs, live.RunID) {
		runs = append(runs, live)
	}
	if len(runs) == 0 {
		projection.view.Status = StatusFailed
		projection.view.Error = "subagents: child run was not recorded"
		return projection, fmt.Errorf("%w: child run was not recorded", ErrTaskNotUsable)
	}

	// SessionView 在 Runner 的记录与账本交接边界内取快照，避免先看到终态 Run、
	// 却仍拿到落账前的旧 Entries，进而过早投递一条没有结果的稳定通知。
	entries := childView.Entries
	for index, run := range runs {
		status, err := taskStatus(run.Status)
		if err != nil {
			return projection, err
		}
		turn := TurnRecord{
			Turn:   index + 1,
			RunID:  run.RunID,
			Status: status,
			Error:  run.Error,
		}
		if status == StatusCompleted {
			turn.ResultEntryID = findFinalAssistantEntry(entries, run.RunID)
			if turn.ResultEntryID != "" {
				result, err := readTurnResult(record.ID, turn, entries)
				if err != nil {
					return projection, err
				}
				projection.view.Results = append(projection.view.Results, result)
			}
		}
		projection.view.Turns = append(projection.view.Turns, turn)
		if terminalTaskStatus(status) {
			projection.notifications = append(projection.notifications, Notification{
				NotificationID: notificationID(record.ID, turn.Turn),
				TaskID:         record.ID,
				ChildSessionID: record.ChildSessionID,
				RunID:          turn.RunID,
				Turn:           turn.Turn,
				Status:         turn.Status,
				ResultEntryID:  turn.ResultEntryID,
				Error:          turn.Error,
			})
		}
	}

	latest := projection.view.Turns[len(projection.view.Turns)-1]
	projection.view.Notifications = append([]Notification(nil), projection.notifications...)
	projection.view.Status = latest.Status
	projection.view.Turn = latest.Turn
	projection.view.CurrentRunID = latest.RunID
	projection.view.ResultEntryID = latest.ResultEntryID
	projection.view.Error = latest.Error
	return projection, nil
}

func containsRun(runs []runner.RunState, runID string) bool {
	for _, run := range runs {
		if run.RunID == runID {
			return true
		}
	}
	return false
}

func taskStatus(status runner.RunStatus) (TaskStatus, error) {
	switch status {
	case runner.RunRunning:
		return StatusRunning, nil
	case runner.RunSucceeded:
		return StatusCompleted, nil
	case runner.RunFailed:
		return StatusFailed, nil
	case runner.RunCancelled:
		return StatusCancelled, nil
	case runner.RunInterrupted:
		return StatusInterrupted, nil
	default:
		return "", fmt.Errorf("subagents: unknown child run status %q", status)
	}
}

func terminalTaskStatus(status TaskStatus) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted:
		return true
	default:
		return false
	}
}

func findFinalAssistantEntry(entries []session.Entry, runID string) string {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Message.RunID != runID || entry.Message.Role != session.RoleAssistant || entry.Message.Incomplete {
			continue
		}
		hasText := false
		for _, block := range entry.Message.Blocks {
			if block.Kind == "tool-call" || block.Tool != nil {
				return ""
			}
			if block.Kind == "text" && strings.TrimSpace(block.Text) != "" {
				hasText = true
			}
		}
		if hasText {
			return entry.ID
		}
		return ""
	}
	return ""
}

func readTurnResult(taskID string, turn TurnRecord, entries []session.Entry) (TurnResult, error) {
	for _, entry := range entries {
		if entry.ID != turn.ResultEntryID {
			continue
		}
		if entry.Message.RunID != turn.RunID || entry.Message.Role != session.RoleAssistant {
			break
		}
		var text strings.Builder
		for _, block := range entry.Message.Blocks {
			if block.Kind == "tool-call" || block.Tool != nil {
				return TurnResult{}, fmt.Errorf("subagents: task %s turn %d result entry contains tool call", taskID, turn.Turn)
			}
			if block.Kind == "text" {
				text.WriteString(block.Text)
			}
		}
		return TurnResult{Turn: turn.Turn, RunID: turn.RunID, EntryID: entry.ID, Text: text.String()}, nil
	}
	return TurnResult{}, fmt.Errorf("subagents: task %s turn %d result entry missing or mismatched", taskID, turn.Turn)
}

func unavailableProjection(err error) bool {
	return errors.Is(err, ErrTaskNotUsable)
}
