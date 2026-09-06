package subagents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"harness/kernel/llm"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// Spawn 派出子任务：查父快照、校验一层委派、原子保存关系与内存发布、创建Session并启动。
func (s *Subagents) Spawn(ctx context.Context, input SpawnInput) (SpawnResult, error) {
	err := ctx.Err()
	if err != nil {
		return SpawnResult{}, err
	}
	if input.ParentSessionID == "" || input.ParentRunID == "" {
		return SpawnResult{}, ErrParentRequired
	}
	if strings.TrimSpace(input.Description) == "" {
		return SpawnResult{}, ErrDescriptionEmpty
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return SpawnResult{}, ErrClosed
	}
	s.inFlight.Add(1)
	if _, isChild := s.childSessions[input.ParentSessionID]; isChild {
		s.mu.RUnlock()
		s.inFlight.Done()
		return SpawnResult{}, ErrNestedDelegation
	}
	s.mu.RUnlock()
	defer s.inFlight.Done()

	// 从可信 SessionID + RunID 读取父 Run 配置快照
	parentSettings, err := s.runner.RunSettings(input.ParentSessionID, input.ParentRunID)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("subagents: get parent run settings: %w", err)
	}

	chosenAgentID := parentSettings.AgentID
	if input.AgentID != "" {
		chosenAgentID = input.AgentID
		agentsList, err := s.agents.List()
		if err != nil {
			return SpawnResult{}, err
		}
		found := false
		for _, a := range agentsList {
			if a.ID == chosenAgentID {
				found = true
				break
			}
		}
		if !found {
			return SpawnResult{}, fmt.Errorf("subagents: agent %q not found", chosenAgentID)
		}
	}

	chosenModel := parentSettings.Model
	if input.Model != "" {
		chosenModel = input.Model
	}

	modelsList := s.models.Models()
	var matchedModel *llm.ModelChoice
	for i := range modelsList {
		if modelsList[i].ID == chosenModel {
			matchedModel = &modelsList[i]
			break
		}
	}
	if matchedModel == nil {
		return SpawnResult{}, fmt.Errorf("subagents: model %q not found or not configured", chosenModel)
	}

	chosenEffort := parentSettings.ReasoningEffort
	if input.ReasoningEffort != "" {
		chosenEffort = input.ReasoningEffort
	}
	if chosenEffort != "" {
		supported := false
		for _, eff := range matchedModel.ReasoningEfforts {
			if eff == chosenEffort {
				supported = true
				break
			}
		}
		if !supported {
			return SpawnResult{}, fmt.Errorf("subagents: model %q does not support reasoning effort %q", chosenModel, chosenEffort)
		}
	}

	workspace := parentSettings.Workspace

	taskID, err := newTaskID()
	if err != nil {
		return SpawnResult{}, err
	}
	childSessionID, err := session.NewID()
	if err != nil {
		return SpawnResult{}, err
	}

	now := time.Now().UTC()
	task := Task{
		Version:         currentTaskVersion,
		ID:              taskID,
		ParentSessionID: input.ParentSessionID,
		ParentRunID:     input.ParentRunID,
		ChildSessionID:  childSessionID,
		AgentID:         chosenAgentID,
		Model:           chosenModel,
		ReasoningEffort: chosenEffort,
		Workspace:       workspace,
		Description:     input.Description,
		Status:          StatusPending,
		Turn:            1,
		Turns: []TurnRecord{
			{
				Turn:      1,
				Status:    StatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	coord := &taskCoord{
		task:         task,
		finalizingCh: make(chan struct{}),
	}
	coord.mu.Lock()
	defer coord.mu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return SpawnResult{}, ErrClosed
	}
	s.childSessions[childSessionID] = taskID
	s.parentTasks[input.ParentSessionID] = append(s.parentTasks[input.ParentSessionID], taskID)
	s.coords[taskID] = coord
	s.mu.Unlock()

	// 关系先落盘，之后才能创建会话。coord.mu 覆盖整个创建过程，
	// 因而 List / Send / 完成回调不会观察或覆盖半成品。
	err = s.store.saveTask(coord.task)
	if err != nil {
		return SpawnResult{}, s.failTurn(coord, fmt.Errorf("%w: save initial task: %w", ErrPersistFailed, err))
	}

	_, err = s.sessions.Create(childSessionID)
	if err != nil {
		return SpawnResult{}, s.failTurn(coord, fmt.Errorf("create child session: %w", err))
	}
	err = s.settings.Put(childSessionID, settings.SessionSettings{
		AgentID:         chosenAgentID,
		Model:           chosenModel,
		ReasoningEffort: chosenEffort,
		Workspace:       workspace,
	})
	if err != nil {
		return SpawnResult{}, s.failTurn(coord, fmt.Errorf("save child settings: %w", err))
	}

	runID, err := s.startTurn(coord, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: input.Description}},
	})
	if err != nil {
		return SpawnResult{}, err
	}
	return SpawnResult{TaskID: taskID, ChildSessionID: childSessionID, RunID: runID}, nil
}
