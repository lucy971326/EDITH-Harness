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
		Skills:       []string{"git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID == "" {
		t.Fatal("Save() did not generate ID")
	}
	choices := service.Choices()
	if len(choices.Loops) != 1 || len(choices.Tools) != 1 || len(choices.Skills) != 1 {
		t.Fatalf("Choices() = %#v", choices)
	}
	prepared, err := service.Prepare(agent.ID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Kind != "react" || !slices.Equal(prepared.Tools, []string{"bash"}) {
		t.Fatalf("PreparedAgent = %#v", prepared)
	}
	wantPrompt := "Work carefully.\n\n## Available Skills\n- git: Commit only tested changes.\n\n## Workspace\n/work"
	if prepared.SystemPrompt != wantPrompt {
		t.Fatalf("SystemPrompt = %q, want %q", prepared.SystemPrompt, wantPrompt)
	}
}

func TestService_defaultAgentUsesNewlyRegisteredToolsAndSkills(t *testing.T) {
	service, loopsRegistry, toolsRegistry, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "read")
	registerSkill(t, skillsRegistry, "first", "First summary.")

	first, err := service.Prepare(DefaultID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	registerTool(t, toolsRegistry, "bash")
	registerSkill(t, skillsRegistry, "second", "Second summary.")
	second, err := service.Prepare(DefaultID, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first.Tools, []string{"read"}) {
		t.Fatalf("first tools = %#v", first.Tools)
	}
	if !slices.Equal(second.Tools, []string{"bash", "read"}) {
		t.Fatalf("second tools = %#v", second.Tools)
	}
	if !strings.Contains(second.SystemPrompt, "- second: Second summary.") {
		t.Fatalf("second prompt = %q", second.SystemPrompt)
	}
}

func TestService_rejectsInvalidConfigurationAndDefaultChanges(t *testing.T) {
	service, loopsRegistry, toolsRegistry, skillsRegistry := testService(t)
	registerLoop(t, loopsRegistry, "react")
	registerTool(t, toolsRegistry, "bash")
	registerSkill(t, skillsRegistry, "git", "Git workflow.")

	for _, agent := range []Agent{
		{Name: "bad", Kind: "missing"},
		{Name: "bad", Kind: "react", Tools: []string{"missing"}},
		{Name: "bad", Kind: "react", Skills: []string{"missing"}},
		{Name: "bad", Kind: "react", Tools: []string{"bash", "bash"}},
	} {
		if _, err := service.Save(agent); err == nil {
			t.Fatalf("Save(%#v) error = nil", agent)
		}
	}
	if _, err := service.Save(Agent{ID: DefaultID, Name: "Default", Kind: "react"}); err == nil {
		t.Fatal("Save(default) error = nil")
	}
	if err := service.Delete(DefaultID); err == nil {
		t.Fatal("Delete(default) error = nil")
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
	service, err := NewService(newMemoryStore(), loopRegistry, toolRegistry, skillRegistry)
	if err != nil {
		t.Fatal(err)
	}
	return service, loopRegistry, toolRegistry, skillRegistry
}

func registerLoop(t *testing.T, registry *loops.Registry, kind string) {
	t.Helper()
	if _, err := registry.Register(agentTestLoop{definition: loops.Definition{Kind: kind, Description: kind + " loop."}}); err != nil {
		t.Fatal(err)
	}
}

func registerTool(t *testing.T, registry *tools.Registry, name string) {
	t.Helper()
	tool := tools.New(name, name+" tool.", func(context.Context, tools.Call, struct{}) (tools.Result, error) {
		return tools.Result{}, nil
	})
	if _, err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
}

func registerSkill(t *testing.T, registry *skills.Registry, name string, summary string) {
	t.Helper()
	if _, err := registry.Register(skills.Skill{Name: name, Summary: summary}); err != nil {
		t.Fatal(err)
	}
}

type agentTestLoop struct {
	definition loops.Definition
}

func (l agentTestLoop) Definition() loops.Definition { return l.definition }

func (agentTestLoop) Run(context.Context, loops.Invocation) error { return nil }

type memoryStore struct {
	agents map[string]Agent
}

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
