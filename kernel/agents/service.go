package agents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"harness/kernel/loops"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

const defaultSystemPrompt = "You are Harness, a helpful assistant."

// 活对象。挂在 Host 的 agents 键上的 Agent 设置服务。
type Service struct {
	store    AgentStore
	settings settings.SessionSettingsStore
	loops    loops.Loops
	tools    tools.Tools
	skills   skills.Skills
}

// NewService 组装 Agent 设置服务。
func NewService(store AgentStore, settingsStore settings.SessionSettingsStore, loopRegistry loops.Loops, toolRegistry tools.Tools, skillRegistry skills.Skills) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("agents: nil store")
	}
	if settingsStore == nil {
		return nil, fmt.Errorf("agents: nil session settings")
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
	service := &Service{store: store, settings: settingsStore, loops: loopRegistry, tools: toolRegistry, skills: skillRegistry}
	if err := service.ensureDefault(); err != nil {
		return nil, err
	}
	return service, nil
}

// List 返回所有 Agent；default 始终排在第一项。
func (s *Service) List() ([]Agent, error) {
	registered, err := s.store.ListAgents()
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(registered))
	for _, agent := range registered {
		if agent.ID == DefaultID {
			out = append([]Agent{agent}, out...)
			continue
		}
		out = append(out, agent)
	}
	return out, nil
}

// Get 按 ID 取一份 Agent 设置。
func (s *Service) Get(id string) (Agent, error) {
	return s.store.ForAgent(id)
}

// Save 新建或更新一份 Agent。空 ID 会生成一个新 ID。
func (s *Service) Save(agent Agent) (Agent, error) {
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

// Delete 删除一份未被会话使用的自建 Agent；默认 Agent 不可删除。
func (s *Service) Delete(id string) error {
	if id == DefaultID {
		return fmt.Errorf("agents: default agent cannot be deleted")
	}
	inUse, err := s.InUse(id)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("agents: agent %q is used by a session", id)
	}
	return s.store.DeleteAgent(id)
}

// InUse 返回指定 Agent 是否仍被某本会话选择。
func (s *Service) InUse(id string) (bool, error) {
	return s.settings.UsesAgent(id)
}

// Choices 返回此刻可被 Agent 选择的 Loop、Tool 和用户级 Skill。
func (s *Service) Choices() (Choices, error) {
	available, err := s.skills.List("")
	if err != nil {
		return Choices{}, err
	}
	return Choices{
		Loops:  s.loops.Definitions(),
		Tools:  s.tools.List(),
		Skills: userSkills(available),
	}, nil
}

// AvailableSkills 返回指定 Agent 在当前工作区可见的 Skill。
func (s *Service) AvailableSkills(id string, workspace string) ([]skills.Skill, error) {
	agent, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return s.availableSkills(agent, workspace)
}

func (s *Service) availableSkills(agent Agent, workspace string) ([]skills.Skill, error) {
	if !hasSkillReader(agent.Tools) {
		return []skills.Skill{}, nil
	}
	discovered, err := s.skills.List(workspace)
	if err != nil {
		return nil, err
	}
	userAvailable := discovered
	if workspace != "" {
		userAvailable, err = s.skills.List("")
		if err != nil {
			return nil, err
		}
	}
	selectedNames := userSkillSelections(agent.Skills, userAvailable)
	selected, err := selectSkills(selectedNames, userAvailable, discovered)
	if err != nil {
		return nil, err
	}

	out := append([]skills.Skill(nil), selected...)
	included := make(map[string]struct{}, len(out))
	for _, skill := range out {
		included[skill.Name] = struct{}{}
	}
	for _, skill := range discovered {
		if skill.Scope != skills.ScopeSystem && skill.Scope != skills.ScopeWorkspace {
			continue
		}
		if _, selected := included[skill.Name]; selected {
			continue
		}
		out = append(out, skill)
	}
	return out, nil
}

func userSkillSelections(names []string, available []skills.Skill) []string {
	system := make(map[string]struct{})
	for _, skill := range available {
		if skill.Scope == skills.ScopeSystem {
			system[skill.Name] = struct{}{}
		}
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, automatic := system[name]; automatic {
			continue
		}
		out = append(out, name)
	}
	return out
}

// Prepare 按当前登记内容生成一轮 Run 可用的 Agent 设置和系统提示词。
func (s *Service) Prepare(ctx context.Context, id string, workspace string) (PreparedAgent, error) {
	agent, err := s.Get(id)
	if err != nil {
		return PreparedAgent{}, err
	}
	if _, err := s.loops.Get(agent.Kind); err != nil {
		return PreparedAgent{}, err
	}
	preparedTools, err := s.tools.Prepare(ctx, workspace, agent.Tools)
	if err != nil {
		return PreparedAgent{}, err
	}

	skillDefinitions, err := s.availableSkills(agent, workspace)
	if err != nil {
		return PreparedAgent{}, err
	}
	return PreparedAgent{
		Kind:         agent.Kind,
		Tools:        preparedTools.Names,
		SystemPrompt: assemblePrompt(agent.SystemPrompt, skillDefinitions, preparedTools.Instructions, workspace, preparedTools.Names),
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
	if _, err := s.tools.Definitions(context.Background(), "", agent.Tools); err != nil {
		return err
	}
	available, err := s.skills.List("")
	if err != nil {
		return err
	}
	if _, err := selectSkills(agent.Skills, available, available); err != nil {
		return err
	}
	return nil
}

func (s *Service) ensureDefault() error {
	_, err := s.store.ForAgent(DefaultID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	choices, err := s.Choices()
	if err != nil {
		return err
	}
	defaults := s.tools.Defaults()
	toolNames := make([]string, 0, len(defaults))
	for _, tool := range defaults {
		toolNames = append(toolNames, tool.Name)
	}
	skillNames := make([]string, 0, len(choices.Skills))
	for _, skill := range choices.Skills {
		skillNames = append(skillNames, skill.Name)
	}
	return s.store.PutAgent(Agent{
		ID:           DefaultID,
		Name:         "Harness",
		Kind:         "react",
		SystemPrompt: defaultSystemPrompt,
		Tools:        toolNames,
		Skills:       skillNames,
	})
}

func assemblePrompt(systemPrompt string, selected []skills.Skill, instructions []tools.Instruction, workspace string, allowedTools []string) string {
	parts := make([]string, 0, 4)
	if text := strings.TrimSpace(systemPrompt); text != "" {
		parts = append(parts, text)
	}
	if len(selected) > 0 {
		lines := make([]string, 0, len(selected)*2+5)
		lines = append(lines, "## Available Skills")
		for _, skill := range selected {
			lines = append(lines, "- "+skill.Name+": "+skill.Description)
			lines = append(lines, "  Location: "+skill.Location)
		}
		lines = append(lines, "", skillReadInstruction(allowedTools), "Resolve relative resources from each Skill directory.", "User instructions take priority over Skill instructions.")
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(instructions) > 0 {
		lines := []string{"## MCP Server Instructions"}
		for _, instruction := range instructions {
			lines = append(lines, "### "+instruction.Source, instruction.Text)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	parts = append(parts, "## Workspace\n"+workspace)
	return strings.Join(parts, "\n\n")
}

func userSkills(available []skills.Skill) []skills.Skill {
	out := make([]skills.Skill, 0, len(available))
	for _, skill := range available {
		if skill.Scope == skills.ScopeUser {
			out = append(out, skill)
		}
	}
	return out
}

func hasSkillReader(allowed []string) bool {
	for _, name := range allowed {
		if name == "read" || name == "bash" {
			return true
		}
	}
	return false
}

func skillReadInstruction(allowed []string) string {
	hasRead := false
	hasBash := false
	for _, name := range allowed {
		hasRead = hasRead || name == "read"
		hasBash = hasBash || name == "bash"
	}
	switch {
	case hasRead && hasBash:
		return "Read the complete SKILL.md with the existing read or bash tool before using the Skill."
	case hasRead:
		return "Read the complete SKILL.md with the existing read tool before using the Skill."
	default:
		return "Read the complete SKILL.md with the existing bash tool before using the Skill."
	}
}

func selectSkills(names []string, userAvailable []skills.Skill, available []skills.Skill) ([]skills.Skill, error) {
	users := make(map[string]skills.Skill, len(userAvailable))
	for _, skill := range userAvailable {
		if skill.Scope == skills.ScopeUser {
			users[skill.Name] = skill
		}
	}
	effective := make(map[string]skills.Skill, len(available))
	for _, skill := range available {
		effective[skill.Name] = skill
	}
	out := make([]skills.Skill, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("skills: %q appears more than once", name)
		}
		seen[name] = struct{}{}
		if _, ok := users[name]; !ok {
			return nil, fmt.Errorf("skills: %q is not a user skill", name)
		}
		skill, ok := effective[name]
		if !ok {
			skill = users[name]
		}
		out = append(out, skill)
	}
	return out, nil
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
