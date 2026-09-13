package subagents

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/session"
)

// Send 向指定孩子追加要求：忙时 Steer，空闲时在同一子 Session 开启新 Run。
func (s *Subagents) Send(ctx context.Context, parentSessionID, parentRunID, taskID string, input session.UserMessage) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}
	if taskID == "" {
		return SendResult{}, fmt.Errorf("subagents: empty task id")
	}
	var text strings.Builder
	for _, block := range input.Blocks {
		if block.Kind != "text" || block.Tool != nil || block.Result != nil || block.Media != nil {
			return SendResult{}, fmt.Errorf("subagents: send accepts only text")
		}
		text.WriteString(block.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return SendResult{}, ErrDescriptionEmpty
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return SendResult{}, ErrClosed
	}
	s.inFlight.Add(1)
	coord := s.coords[taskID]
	var stopGeneration uint64
	if coord != nil {
		stopGeneration = coord.stopGeneration
	}
	s.mu.RUnlock()
	defer s.inFlight.Done()
	if coord == nil {
		return SendResult{}, ErrTaskNotFound
	}

	permit, err := s.admit(parentSessionID, parentRunID)
	if err != nil {
		return SendResult{}, err
	}

	coord.mu.Lock()
	defer coord.mu.Unlock()
	if coord.record.ParentSessionID != parentSessionID {
		return SendResult{}, ErrOwnershipMismatch
	}
	s.mu.RLock()
	err = s.admissionErrorLocked(permit)
	childStopped := coord.stopRequested
	crossedChildStop := coord.stopGeneration != stopGeneration
	s.mu.RUnlock()
	if err != nil {
		return SendResult{}, err
	}
	if crossedChildStop {
		return SendResult{}, ErrTaskStopped
	}

	if live, active := s.runner.State(coord.record.ChildSessionID); active {
		if childStopped {
			return SendResult{}, ErrTaskStopped
		}
		projection, projectErr := s.projectTask(coord.record)
		if projectErr != nil {
			return SendResult{}, projectErr
		}
		turn := projection.view.Turn
		childSessionID := coord.record.ChildSessionID
		coord.mu.Unlock()
		err = s.runner.SteerRun(childSessionID, live.RunID, input)
		coord.mu.Lock()
		if err != nil {
			// Steer 可能已落账，错误时不盲目重发。
			return SendResult{}, fmt.Errorf("subagents: steer child run: %w", err)
		}
		return SendResult{Turn: turn, RunID: live.RunID, Steered: true}, nil
	}

	// 子 Session 和设置是启动新 Run 的真实资源。
	_, err = s.sessions.Get(coord.record.ChildSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}
	_, err = s.settings.For(coord.record.ChildSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}
	childView, err := s.runner.SessionView(coord.record.ChildSessionID)
	if err != nil {
		return SendResult{}, err
	}
	newTurn := len(childView.Runs) + 1

	s.mu.Lock()
	err = s.admissionErrorLocked(permit)
	if err == nil && coord.stopGeneration != stopGeneration {
		err = ErrTaskStopped
	}
	if err == nil {
		coord.admission = permit
		coord.stopRequested = false
	}
	s.mu.Unlock()
	if err != nil {
		return SendResult{}, err
	}

	runID, err := s.startTurn(coord, input)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{Turn: newTurn, RunID: runID}, nil
}
