package agents

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

func TestService_saveChoicesAndPrepare(t *testing.T) {
	service, loopsRegistry, toolsRegistry, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "bash")
	registerSkill(t, skillsRegistry, "git", "Commit only tested changes.")

	agent, err := service.Save(Agent{
		Name:         "Coding",
		Kind:         "react",
		SystemPrompt: "Work carefully.",
		Tools:        []string{"bash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID == "" {
		t.Fatal("Save() did not generate ID")
	}
	choices := service.Choices()
	if len(choices.Loops) != 1 || len(choices.Tools) != 1 {
		t.Fatalf("Choices() = %#v", choices)
	}
	prepared, err := service.Prepare(context.Background(), agent.ID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Kind != "react" || !slices.Equal(prepared.Tools, []string{"bash"}) {
		t.Fatalf("PreparedAgent = %#v", prepared)
	}
	wantPrompt := "Work carefully.\n\n## Available Skills\n- git: Commit only tested changes.\n  Location: /skills/git/SKILL.md\n\nRead the complete SKILL.md with the existing bash tool before using the Skill.\nResolve relative resources from each Skill directory.\nUser instructions take priority over Skill instructions.\n\n## Workspace\n/work"
	if prepared.SystemPrompt != wantPrompt {
		t.Fatalf("SystemPrompt = %q, want %q", prepared.SystemPrompt, wantPrompt)
	}
}

func TestService_defaultAgentKeepsInitialToolsAndAddsNewSkills(t *testing.T) {
	loopsRegistry := loops.NewRegistry()
	toolsRegistry := tools.NewRegistry()
	skillsRegistry := skills.NewRegistry()
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "read")
	registerSkill(t, skillsRegistry, "first", "First summary.")
	service, err := NewService(newMemoryStore(), noAgentUse{}, loopsRegistry, toolsRegistry, skillsRegistry)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Prepare(context.Background(), DefaultID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	registerTool(t, toolsRegistry, "bash")
	registerSkill(t, skillsRegistry, "second", "Second summary.")
	second, err := service.Prepare(context.Background(), DefaultID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first.Tools, []string{"read"}) {
		t.Fatalf("first tools = %#v", first.Tools)
	}
	if !slices.Equal(second.Tools, []string{"read"}) {
		t.Fatalf("second tools = %#v", second.Tools)
	}
	if !strings.Contains(second.SystemPrompt, "- first: First summary.") || !strings.Contains(second.SystemPrompt, "- second: Second summary.") {
		t.Fatalf("second prompt = %q", second.SystemPrompt)
	}
}

func TestService_rejectsInvalidConfigurationAndAllowsDefaultChanges(t *testing.T) {
	service, loopsRegistry, toolsRegistry, _ := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "bash")

	for _, agent := range []Agent{
		{Name: "bad", Kind: "missing"},
		{Name: "bad", Kind: "react", Tools: []string{"missing"}},
		{Name: "bad", Kind: "react", Tools: []string{"bash", "bash"}},
	} {
		if _, err := service.Save(agent); err == nil {
			t.Fatalf("Save(%#v) error = nil", agent)
		}
	}
	updated, err := service.Save(Agent{ID: DefaultID, Name: "Harness", Kind: "react", Tools: []string{"bash"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(updated.Tools, []string{"bash"}) {
		t.Fatalf("updated default = %#v", updated)
	}
	if err := service.Delete(DefaultID); err == nil {
		t.Fatal("Delete(default) error = nil")
	}
}

func TestService_prepareIncludesAllScopedSkillsWithoutSelecting(t *testing.T) {
	service, loopsRegistry, toolsRegistry, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "read")
	err := skillsRegistry.Register(testSkillListProvider{skills: func(workspace string) []skills.Skill {
		out := []skills.Skill{
			{Name: "system-skill", Description: "System skill.", Location: "/system/skills/system-skill/SKILL.md", Scope: skills.ScopeSystem},
			{Name: "user-skill", Description: "User skill.", Location: "/home/skills/user-skill/SKILL.md", Scope: skills.ScopeUser},
		}
		if workspace != "" {
			out = append(out, skills.Skill{Name: "project-skill", Description: "Project skill.", Location: "/work/.harness/skills/project-skill/SKILL.md", Scope: skills.ScopeWorkspace})
		}
		return out
	}})
	if err != nil {
		t.Fatal(err)
	}
	choices := service.Choices()
	if len(choices.Tools) != 1 || choices.Tools[0].Name != "read" {
		t.Fatalf("Choices() = %#v", choices)
	}
	agent, err := service.Save(Agent{ID: "coding", Name: "Coding", Kind: "react", Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(context.Background(), agent.ID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- user-skill: User skill.",
		"Location: /home/skills/user-skill/SKILL.md",
		"- system-skill: System skill.",
		"Location: /system/skills/system-skill/SKILL.md",
		"- project-skill: Project skill.",
		"Location: /work/.harness/skills/project-skill/SKILL.md",
		"existing read tool",
		"Resolve relative resources from each Skill directory.",
		"User instructions take priority over Skill instructions.",
	} {
		if !strings.Contains(prepared.SystemPrompt, want) {
			t.Fatalf("SystemPrompt = %q, missing %q", prepared.SystemPrompt, want)
		}
	}
}

func TestService_prepareWithoutSkillReaderStillListsSkills(t *testing.T) {
	service, loopsRegistry, _, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerSkill(t, skillsRegistry, "git", "Git workflow.")
	agent, err := service.Save(Agent{ID: "plain", Name: "Plain", Kind: "react"})
	if err != nil {
		t.Fatal(err)
	}
	available, err := service.AvailableSkills("/work")
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].Name != "git" {
		t.Fatalf("AvailableSkills() = %#v", available)
	}
	prepared, err := service.Prepare(context.Background(), agent.ID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prepared.SystemPrompt, "- git: Git workflow.") {
		t.Fatalf("SystemPrompt = %q", prepared.SystemPrompt)
	}
	if strings.Contains(prepared.SystemPrompt, "existing") || slices.Contains(prepared.Tools, "read") || slices.Contains(prepared.Tools, "bash") {
		t.Fatalf("PreparedAgent = %#v", prepared)
	}
}

func TestService_availableSkillsIgnoresAgentAndStripsLegacyTools(t *testing.T) {
	service, loopsRegistry, toolsRegistry, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "read")
	err := skillsRegistry.Register(testSkillListProvider{skills: func(string) []skills.Skill {
		return []skills.Skill{{
			Name: "skill-creator", Description: "System skill.",
			Location: "/system/skill-creator/SKILL.md", Scope: skills.ScopeSystem,
		}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	agent := Agent{ID: "legacy", Name: "Legacy", Kind: "react", Tools: []string{"read", "mcp__tavily__search"}}
	err = service.store.PutAgent(agent)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Tools, []string{"read"}) {
		t.Fatalf("Get() = %#v", got)
	}
	available, err := service.AvailableSkills("/work")
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 1 || available[0].Name != "skill-creator" {
		t.Fatalf("AvailableSkills() = %#v", available)
	}
}

func TestService_availableSkillsPropagatesDiscoveryError(t *testing.T) {
	service, loopsRegistry, _, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	err := skillsRegistry.Register(testSkillErrorProvider{err: errors.New("broken skill root")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AvailableSkills("/work")
	if err == nil || !strings.Contains(err.Error(), "broken skill root") {
		t.Fatalf("AvailableSkills() error = %v", err)
	}
}

func TestService_deleteRejectsAgentUsedBySession(t *testing.T) {
	loopRegistry := loops.NewRegistry()
	toolRegistry := tools.NewRegistry()
	skillRegistry := skills.NewRegistry()
	registerLoop(t, loopRegistry, "react")
	usage := &agentUse{used: map[string]bool{"coding": true}}
	service, err := NewService(newMemoryStore(), usage, loopRegistry, toolRegistry, skillRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(Agent{ID: "coding", Name: "Coding", Kind: "react"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete("coding"); err == nil {
		t.Fatal("Delete(coding) error = nil")
	}
	usage.used["coding"] = false
	if err := service.Delete("coding"); err != nil {
		t.Fatal(err)
	}
}

func TestPlugin_registersAgentService(t *testing.T) {
	h := host.NewHost()
	if err := h.Install(&persist.Plugin{Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(loops.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(tools.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(skills.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(NewPlugin()); err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	service, err := host.Resolve[*Service](h, "agents")
	if err != nil {
		t.Fatal(err)
	}
	if service == nil {
		t.Fatal("agents service = nil")
	}
}

func testService(t *testing.T) (*Service, *loops.Registry, *tools.Registry, *skills.Registry) {
	t.Helper()
	loopRegistry := loops.NewRegistry()
	toolRegistry := tools.NewRegistry()
	skillRegistry := skills.NewRegistry()
	service, err := NewService(newMemoryStore(), noAgentUse{}, loopRegistry, toolRegistry, skillRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return service, loopRegistry, toolRegistry, skillRegistry
}

func registerLoop(t *testing.T, registry *loops.Registry, kind string) {
	t.Helper()
	err := registry.Register(agentTestLoop{definition: loops.Definition{Kind: kind, Description: kind + " loop."}})
	if err != nil {
		t.Fatal(err)
	}
}

func registerTool(t *testing.T, registry *tools.Registry, name string) {
	t.Helper()
	tool := tools.New(name, name+" tool.", func(context.Context, tools.Call, struct{}) (tools.Result, error) {
		return tools.Result{}, nil
	})
	err := registry.Register(tool)
	if err != nil {
		t.Fatal(err)
	}
}

func registerSkill(t *testing.T, registry *skills.Registry, name string, summary string) {
	t.Helper()
	err := registry.Register(testSkillProvider{skill: skills.Skill{Name: name, Description: summary, Location: "/skills/" + name + "/SKILL.md", Scope: skills.ScopeUser}})
	if err != nil {
		t.Fatal(err)
	}
}

type testSkillProvider struct {
	skill skills.Skill
}

func (p testSkillProvider) Name() string { return p.skill.Name }

func (p testSkillProvider) List(string) ([]skills.Skill, error) {
	return []skills.Skill{p.skill}, nil
}

type testSkillListProvider struct {
	skills func(workspace string) []skills.Skill
}

func (testSkillListProvider) Name() string { return "list" }

func (p testSkillListProvider) List(workspace string) ([]skills.Skill, error) {
	return p.skills(workspace), nil
}

type testSkillErrorProvider struct {
	err error
}

func (testSkillErrorProvider) Name() string { return "error" }

func (p testSkillErrorProvider) List(string) ([]skills.Skill, error) {
	return nil, p.err
}

type agentTestLoop struct {
	definition loops.Definition
}

func (l agentTestLoop) Definition() loops.Definition { return l.definition }

func (agentTestLoop) Run(context.Context, loops.Invocation) error { return nil }

type memoryStore struct {
	agents map[string]Agent
}

type noAgentUse struct{}

func (noAgentUse) For(string) (settings.SessionSettings, error) {
	return settings.SessionSettings{}, nil
}

func (noAgentUse) Put(string, settings.SessionSettings) error { return nil }

func (noAgentUse) UsesAgent(string) (bool, error) { return false, nil }

type agentUse struct {
	used map[string]bool
}

func (s *agentUse) For(string) (settings.SessionSettings, error) {
	return settings.SessionSettings{}, nil
}

func (s *agentUse) Put(string, settings.SessionSettings) error { return nil }

func (s *agentUse) UsesAgent(id string) (bool, error) { return s.used[id], nil }

func newMemoryStore() *memoryStore {
	return &memoryStore{agents: make(map[string]Agent)}
}

func (s *memoryStore) ListAgents() ([]Agent, error) {
	out := make([]Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		out = append(out, copyAgent(agent))
	}
	return out, nil
}

func (s *memoryStore) ForAgent(id string) (Agent, error) {
	agent, ok := s.agents[id]
	if !ok {
		return Agent{}, os.ErrNotExist
	}
	return copyAgent(agent), nil
}

func (s *memoryStore) PutAgent(agent Agent) error {
	s.agents[agent.ID] = copyAgent(agent)
	return nil
}

func (s *memoryStore) DeleteAgent(id string) error {
	if _, ok := s.agents[id]; !ok {
		return errors.New("missing agent")
	}
	delete(s.agents, id)
	return nil
}
