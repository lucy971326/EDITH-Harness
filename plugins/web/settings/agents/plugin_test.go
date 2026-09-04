package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	kernelagents "harness/kernel/agents"
	"harness/kernel/host"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/tools"
	"harness/surface/web"
)

type mockWeb struct {
	sections []web.SettingsSection
	routes   map[string]http.Handler
}

func newMockWeb() *mockWeb { return &mockWeb{routes: make(map[string]http.Handler)} }

func (m *mockWeb) RegisterProduct(web.Product) error { return nil }
func (m *mockWeb) Products() []web.Product           { return nil }
func (m *mockWeb) RegisterRoute(pattern string, handler http.Handler) error {
	m.routes[pattern] = handler
	return nil
}
func (m *mockWeb) RegisterSettingsSection(section web.SettingsSection) error {
	m.sections = append(m.sections, section)
	return nil
}
func (m *mockWeb) SettingsSections() []web.SettingsSectionDefinition { return nil }
func (m *mockWeb) SettingsSection(string) (web.SettingsSection, bool) {
	return nil, false
}

func TestPluginManagesAgentsAndProtectsUsedAgent(t *testing.T) {
	h := host.NewHost()
	if err := h.Install(&persist.Plugin{Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(loops.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		t.Fatal(err)
	}
	if err := loopRegistry.Register(testLoop{}); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(tools.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(skills.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(kernelagents.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	mock := newMockWeb()
	if err := h.RegisterService("web", web.Service(mock)); err != nil {
		t.Fatal(err)
	}
	p := New()
	if err := p.Start(h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })

	if len(mock.sections) != 1 || mock.sections[0].Definition().ID != "agents" {
		t.Fatalf("sections = %#v", mock.sections)
	}
	component, err := mock.sections[0].Render()
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	if err := component.Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "Harness") || !strings.Contains(rendered.String(), "不可删除") {
		t.Fatalf("initial section = %s", rendered.String())
	}
	newHandler := mock.routes["GET /settings/agents/{agentID}"]
	newRequest := httptest.NewRequest(http.MethodGet, "/settings/agents/new", nil)
	newRequest.SetPathValue("agentID", "new")
	newResponse := httptest.NewRecorder()
	newHandler.ServeHTTP(newResponse, newRequest)
	if newResponse.Code != http.StatusOK || !strings.Contains(newResponse.Body.String(), "Harness") || !strings.Contains(newResponse.Body.String(), "新建 Agent") {
		t.Fatalf("new status=%d body=%s", newResponse.Code, newResponse.Body.String())
	}

	create := mock.routes["POST /settings/agents"]
	request := httptest.NewRequest(http.MethodPost, "/settings/agents", strings.NewReader(url.Values{
		"name": {"Coding"}, "kind": {"react"},
	}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	create.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Coding") {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}

	service, err := host.Resolve[*kernelagents.Service](h, "agents")
	if err != nil {
		t.Fatal(err)
	}
	all, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("agents = %#v", all)
	}
	codingID := all[1].ID
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	if err := settingsStore.Put("session-1", settings.SessionSettings{AgentID: codingID}); err != nil {
		t.Fatal(err)
	}
	deleteHandler := mock.routes["POST /settings/agents/{agentID}/delete"]
	deleteRequest := httptest.NewRequest(http.MethodPost, "/settings/agents/"+codingID+"/delete", nil)
	deleteRequest.SetPathValue("agentID", codingID)
	deleteResponse := httptest.NewRecorder()
	deleteHandler.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusBadRequest || !strings.Contains(deleteResponse.Body.String(), "used by a session") {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
}

type testLoop struct{}

func (testLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "test loop"}
}

func (testLoop) Run(context.Context, loops.Invocation) error { return nil }
