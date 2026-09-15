package subagents

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/session"
)

// Send 由父 Agent 向孩子追加文字；调用必须仍属于一个活跃的父 Run。
func (s *Subagents) Send(ctx context.Context, parentSessionID, parentRunID, taskID string, input session.UserMessage) (SendResult, error) {
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
	permit, err := s.admit(parentSessionID, parentRunID)
	if err != nil {
		return SendResult{}, err
	}
	return s.sendAccepted(ctx, parentSessionID, taskID, input, permit)
}

// SendFromUser 由用户从 Subagent 页面续聊；父 Run 可以已经结束。
func (s *Subagents) SendFromUser(ctx context.Context, parentSessionID, taskID string, input session.UserMessage) (SendResult, error) {
	if !validUserMessage(input) {
		return SendResult{}, ErrDescriptionEmpty
	}

	s.mu.RLock()
	coord := s.coords[taskID]
	if s.closed {
		s.mu.RUnlock()
		return SendResult{}, ErrClosed
	}
	if coord == nil || coord.record.ParentSessionID != parentSessionID {
		s.mu.RUnlock()
		return SendResult{}, ErrTaskNotFound
	}
	childSessionID := coord.record.ChildSessionID
	permit := admission{parentSessionID: parentSessionID, generation: s.families[parentSessionID].generation}
	s.mu.RUnlock()
	setup, err := s.settings.For(childSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}

	for _, block := range input.Blocks {
		if block.Kind == "image" && !s.models.Vision(setup.Model) {
			return SendResult{}, fmt.Errorf("subagents: current model does not support images")
		}
	}
	return s.sendAccepted(ctx, parentSessionID, taskID, input, permit)
}

func validUserMessage(input session.UserMessage) bool {
	for _, block := range input.Blocks {
		switch block.Kind {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return true
			}
		case "image":
			if block.Media != nil && block.Media.MIME != "" && block.Media.Data != "" {
				return true
			}
		}
	}
	return false
}

func (s *Subagents) sendAccepted(ctx context.Context, parentSessionID, taskID string, input session.UserMessage, permit admission) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}
	if taskID == "" {
		return SendResult{}, fmt.Errorf("subagents: empty task id")
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
		err = s.runner.SteerRunContext(ctx, childSessionID, live.RunID, input)
		coord.mu.Lock()
		if err != nil {
			return SendResult{}, fmt.Errorf("subagents: steer child run: %w", err)
		}
		return SendResult{Turn: turn, RunID: live.RunID, Steered: true}, nil
	}

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
