package subagents

import (
	"context"
	"fmt"
	"strings"

	"harness/internal/session"
)

// 数据。一项 Send 在停止边界内取得的父协作与目标任务准入快照。
type sendAdmission struct {
	family         admission
	coord          *taskCoord
	stopGeneration uint64
}

// Send 由父 Agent 向孩子追加文字；调用必须仍属于一个活跃的父 Run。
func (s *Subagents) Send(ctx context.Context, parentSessionID, parentRunID, taskID string, input session.UserMessage) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
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
	if parentSessionID == "" || parentRunID == "" {
		return SendResult{}, ErrParentRequired
	}
	permit, err := s.beginSend(parentSessionID, parentRunID, taskID)
	if err != nil {
		return SendResult{}, err
	}
	defer s.work.Done()
	input.SourceSessionID = parentSessionID
	input.SourceRunID = parentRunID
	return s.sendAccepted(ctx, input, permit)
}

// SendFromUser 由用户从 Subagent 页面续聊；父 Run 可以已经结束。
func (s *Subagents) SendFromUser(ctx context.Context, parentSessionID, taskID string, input session.UserMessage) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}
	if !validUserMessage(input) {
		return SendResult{}, ErrDescriptionEmpty
	}

	permit, err := s.beginSend(parentSessionID, "", taskID)
	if err != nil {
		return SendResult{}, err
	}
	defer s.work.Done()
	setup, err := s.settings.For(permit.coord.record.ChildSessionID)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %v", ErrTaskNotUsable, err)
	}

	for _, block := range input.Blocks {
		if block.Kind == "image" && !s.models.Vision(setup.Model) {
			return SendResult{}, fmt.Errorf("subagents: current model does not support images")
		}
	}
	return s.sendAccepted(ctx, input, permit)
}

// beginSend 在线性停止边界内同时取得父协作代次、目标任务和任务停止代次。
// parentRunID 为空表示用户从工作页续聊，不要求直属父 Run 仍活跃。
func (s *Subagents) beginSend(parentSessionID, parentRunID, taskID string) (sendAdmission, error) {
	if parentSessionID == "" {
		return sendAdmission{}, ErrParentRequired
	}
	if taskID == "" {
		return sendAdmission{}, fmt.Errorf("subagents: empty task id")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ctx.Err() != nil {
		return sendAdmission{}, ErrClosed
	}
	family := s.families[parentSessionID]
	if parentRunID != "" {
		if family.stoppedRunID == parentRunID {
			return sendAdmission{}, ErrFamilyStopped
		}
		state, active := s.runner.State(parentSessionID)
		if !active || state.RunID != parentRunID {
			return sendAdmission{}, ErrParentRequired
		}
	}
	coord := s.coords[taskID]
	if coord == nil {
		return sendAdmission{}, ErrTaskNotFound
	}
	if coord.record.ParentSessionID != parentSessionID {
		return sendAdmission{}, ErrOwnershipMismatch
	}
	s.work.Add(1)
	return sendAdmission{
		family: admission{
			parentSessionID: parentSessionID,
			parentRunID:     parentRunID,
			generation:      family.generation,
		},
		coord:          coord,
		stopGeneration: coord.stopGeneration,
	}, nil
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

func (s *Subagents) sendAccepted(ctx context.Context, input session.UserMessage, permit sendAdmission) (SendResult, error) {
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}

	coord := permit.coord
	coord.mu.Lock()
	defer coord.mu.Unlock()
	s.mu.RLock()
	err = s.admissionErrorLocked(permit.family)
	childStopped := coord.stopRequested
	crossedChildStop := coord.stopGeneration != permit.stopGeneration
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
	err = s.admissionErrorLocked(permit.family)
	if err == nil && coord.stopGeneration != permit.stopGeneration {
		err = ErrTaskStopped
	}
	if err == nil {
		coord.admission = permit.family
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
