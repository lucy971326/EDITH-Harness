package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"harness/kernel/loops"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

const defaultSystemPrompt = "You are Harness, a helpful assistant."

// 活对象。挂在 Host 的 agents 键上的 Agent 设置服务。
type Service struct {
	store  AgentStore
	loops  loops.Loops
	tools  tools.Tools
	skills skills.Skills
}

// NewService 组装 Agent 设置服务。
func NewService(store AgentStore, loopRegistry loops.Loops, toolRegistry tools.Tools, skillRegistry skills.Skills) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("agents: nil store")
	}
	if loopRegistry == nil {
		return nil, fmt.Errorf("agents: nil loops")
	}
	if toolRegistry == nil {
		return nil, fmt.Errorf("agents: nil tools")
	}
	if skillRegistry == nil {
		return nil, fmt.Errorf("agents: nil skills")
	}
	return &Service{store: store, loops: loopRegistry, tools: toolRegistry, skills: skillRegistry}, nil
}

// List 返回默认 Agent 和所有自建 Agent；默认项始终排在第一项。
func (s *Service) List() ([]Agent, error) {
	custom, err := s.store.ListAgents()
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(custom)+1)
	out = append(out, defaultAgent())
	out = append(out, custom...)
	return out, nil
}

// Get 按 ID 取一份 Agent 设置。
func (s *Service) Get(id string) (Agent, error) {
	if id == DefaultID {
		return defaultAgent(), nil
	}
	return s.store.ForAgent(id)
}

// Save 新建或更新自建 Agent。空 ID 会生成一个新 ID。
func (s *Service) Save(agent Agent) (Agent, error) {
	if agent.ID == DefaultID {
		return Agent{}, fmt.Errorf("agents: default agent is read-only")
	}
	if agent.ID == "" {
		id, err := newID()
		if err != nil {
			return Agent{}, err
		}
		agent.ID = id
	}
	if err := s.validate(agent); err != nil {
		return Agent{}, err
	}
	if err := s.store.PutAgent(copyAgent(agent)); err != nil {
		return Agent{}, err
	}
	return copyAgent(agent), nil
}

// Delete 删除一份自建 Agent；默认 Agent 不可删除。
func (s *Service) Delete(id string) error {
	if id == DefaultID {
		return fmt.Errorf("agents: default agent cannot be deleted")
	}
	return s.store.DeleteAgent(id)
}

// Choices 返回此刻可被 Agent 选择的 Loop、Tool 和 Skill。
func (s *Service) Choices() Choices {
	return Choices{
		Loops:  s.loops.Definitions(),
		Tools:  s.tools.List(),
		Skills: s.skills.List(),
	}
}

// Prepare 按当前登记内容生成一轮 Run 可用的 Agent 设置和系统提示词。
func (s *Service) Prepare(id string, workspace string) (PreparedAgent, error) {
	agent, err := s.Get(id)
	if err != nil {
		return PreparedAgent{}, err
	}
	if id == DefaultID {
		definitions := s.tools.List()
		agent.Tools = make([]string, 0, len(definitions))
		for _, definition := range definitions {
			agent.Tools = append(agent.Tools, definition.Name)
		}
		skillDefinitions := s.skills.List()
		agent.Skills = make([]string, 0, len(skillDefinitions))
		for _, skill := range skillDefinitions {
			agent.Skills = append(agent.Skills, skill.Name)
		}
	}
	if _, err := s.loops.Get(agent.Kind); err != nil {
		return PreparedAgent{}, err
	}
	if _, err := s.tools.Definitions(agent.Tools); err != nil {
		return PreparedAgent{}, err
	}
	skillDefinitions, err := s.skills.Get(agent.Skills)
	if err != nil {
		return PreparedAgent{}, err
	}
	return PreparedAgent{
		Kind:         agent.Kind,
		Tools:        append([]string(nil), agent.Tools...),
		SystemPrompt: assemblePrompt(agent.SystemPrompt, skillDefinitions, workspace),
	}, nil
}

func (s *Service) validate(agent Agent) error {
	if strings.TrimSpace(agent.ID) == "" {
		return fmt.Errorf("agents: empty id")
	}
	if strings.TrimSpace(agent.Name) == "" {
		return fmt.Errorf("agents: agent %q has empty name", agent.ID)
	}
	if strings.TrimSpace(agent.Kind) == "" {
		return fmt.Errorf("agents: agent %q has empty kind", agent.ID)
	}
	if _, err := s.loops.Get(agent.Kind); err != nil {
		return err
	}
	if _, err := s.tools.Definitions(agent.Tools); err != nil {
		return err
	}
	if _, err := s.skills.Get(agent.Skills); err != nil {
		return err
	}
	return nil
}

func defaultAgent() Agent {
	return Agent{
		ID:           DefaultID,
		Name:         "Default",
		Kind:         "react",
		SystemPrompt: defaultSystemPrompt,
	}
}

func assemblePrompt(systemPrompt string, selected []skills.Skill, workspace string) string {
	parts := make([]string, 0, 3)
	if text := strings.TrimSpace(systemPrompt); text != "" {
		parts = append(parts, text)
	}
	if len(selected) > 0 {
		lines := make([]string, 0, len(selected)+1)
		lines = append(lines, "## Available Skills")
		for _, skill := range selected {
			lines = append(lines, "- "+skill.Name+": "+skill.Summary)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	parts = append(parts, "## Workspace\n"+workspace)
	return strings.Join(parts, "\n\n")
}

func copyAgent(agent Agent) Agent {
	agent.Tools = append([]string(nil), agent.Tools...)
	agent.Skills = append([]string(nil), agent.Skills...)
	return agent
}

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("agents: make id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
