package appserver

import (
	"context"
	"errors"
	"fmt"
	"os"

	"harness/kernel/agents"
)

const (
	agentListMethod   = "agent/list"
	agentSaveMethod   = "agent/save"
	agentDeleteMethod = "agent/delete"
)

// 数据。Agent 目录不接受过滤参数。
type AgentListParams struct{}

// 数据。对外 Agent 设置及其删除保护状态。
type AgentView struct {
	ID           string   `json:"id" jsonschema:"minLength=1"`
	Name         string   `json:"name" jsonschema:"minLength=1"`
	Kind         string   `json:"kind" jsonschema:"minLength=1"`
	SystemPrompt string   `json:"systemPrompt"`
	Tools        []string `json:"tools"`
	InUse        bool     `json:"inUse"`
}

// 数据。Agent 可选的一种执行类型。
type AgentKindChoice struct {
	Kind        string `json:"kind" jsonschema:"minLength=1"`
	Description string `json:"description"`
}

// 数据。Agent 可选的一个普通工具。
type AgentToolChoice struct {
	Name        string `json:"name" jsonschema:"minLength=1"`
	Description string `json:"description"`
}

// 数据。Agent 列表连同当前可用的执行类型与工具。
type AgentListResult struct {
	Agents []AgentView       `json:"agents"`
	Kinds  []AgentKindChoice `json:"kinds"`
	Tools  []AgentToolChoice `json:"tools"`
}

// 数据。空 ID 新建 Agent，非空 ID 更新 Agent。
type AgentSaveParams struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name" jsonschema:"minLength=1"`
	Kind         string   `json:"kind" jsonschema:"minLength=1"`
	SystemPrompt string   `json:"systemPrompt"`
	Tools        []string `json:"tools"`
}

// 数据。保存后返回后台的真实 Agent 设置。
type AgentSaveResult struct {
	Agent AgentView `json:"agent"`
}

// 数据。删除一个自建 Agent。
type AgentDeleteParams struct {
	AgentID string `json:"agentID" jsonschema:"minLength=1"`
}

// 数据。删除成功返回空对象。
type AgentDeleteResult struct{}

// BindAgents 显式接入公共 Agent 服务，不经产品转发。
func (s *Server) BindAgents(service *agents.Service) error {
	if service == nil || s.agents != nil {
		return fmt.Errorf("appserver: nil or already bound agents")
	}
	s.agents = service
	if err := Register(s, agentListMethod, s.handleAgentList); err != nil {
		return err
	}
	if err := Register(s, agentSaveMethod, s.handleAgentSave); err != nil {
		return err
	}
	return Register(s, agentDeleteMethod, s.handleAgentDelete)
}

func (s *Server) handleAgentList(_ context.Context, _ AgentListParams) (AgentListResult, error) {
	configured, err := s.agents.List()
	if err != nil {
		return AgentListResult{}, err
	}
	result := AgentListResult{
		Agents: make([]AgentView, 0, len(configured)),
		Kinds:  []AgentKindChoice{},
		Tools:  []AgentToolChoice{},
	}
	for _, agent := range configured {
		view, err := s.agentView(agent)
		if err != nil {
			return AgentListResult{}, err
		}
		result.Agents = append(result.Agents, view)
	}
	choices := s.agents.Choices()
	for _, choice := range choices.Loops {
		result.Kinds = append(result.Kinds, AgentKindChoice{Kind: choice.Kind, Description: choice.Description})
	}
	for _, choice := range choices.Tools {
		result.Tools = append(result.Tools, AgentToolChoice{Name: choice.Name, Description: choice.Description})
	}
	return result, nil
}

func (s *Server) handleAgentSave(_ context.Context, input AgentSaveParams) (AgentSaveResult, error) {
	if input.ID != "" {
		_, err := s.agents.Get(input.ID)
		if err != nil {
			return AgentSaveResult{}, agentMethodError(err)
		}
	}
	saved, err := s.agents.Save(agents.Agent{
		ID:           input.ID,
		Name:         input.Name,
		Kind:         input.Kind,
		SystemPrompt: input.SystemPrompt,
		Tools:        input.Tools,
	})
	if err != nil {
		return AgentSaveResult{}, agentMethodError(err)
	}
	view, err := s.agentView(saved)
	return AgentSaveResult{Agent: view}, agentMethodError(err)
}

func (s *Server) handleAgentDelete(_ context.Context, input AgentDeleteParams) (AgentDeleteResult, error) {
	err := s.agents.Delete(input.AgentID)
	return AgentDeleteResult{}, agentMethodError(err)
}

func (s *Server) agentView(agent agents.Agent) (AgentView, error) {
	inUse, err := s.agents.InUse(agent.ID)
	if err != nil {
		return AgentView{}, err
	}
	tools := make([]string, len(agent.Tools))
	copy(tools, agent.Tools)
	return AgentView{
		ID:           agent.ID,
		Name:         agent.Name,
		Kind:         agent.Kind,
		SystemPrompt: agent.SystemPrompt,
		Tools:        tools,
		InUse:        inUse,
	}, nil
}

func agentMethodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, agents.ErrInvalid) {
		return &Error{Code: CodeInvalidParams, Message: "agent settings are invalid", Cause: err}
	}
	if errors.Is(err, agents.ErrDefaultDelete) || errors.Is(err, agents.ErrInUse) {
		return &Error{Code: CodeConflict, Message: "agent cannot be deleted", Cause: err}
	}
	if errors.Is(err, os.ErrNotExist) {
		return &Error{Code: CodeNotFound, Message: "agent not found", Cause: err}
	}
	return err
}
