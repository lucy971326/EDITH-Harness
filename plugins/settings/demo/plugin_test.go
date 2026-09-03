package demo

import (
	"context"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/surface/web"
)

type mockWebService struct {
	sections []web.SettingsSection
	routes   map[string]nethttp.Handler
}

func newMockWebService() *mockWebService {
	return &mockWebService{routes: make(map[string]nethttp.Handler)}
}

func (m *mockWebService) RegisterProduct(web.Product) error { return nil }
func (m *mockWebService) Products() []web.Product           { return nil }

func (m *mockWebService) RegisterRoute(pattern string, handler nethttp.Handler) error {
	m.routes[pattern] = handler
	return nil
}

func (m *mockWebService) RegisterSettingsSection(section web.SettingsSection) error {
	m.sections = append(m.sections, section)
	return nil
}

func (m *mockWebService) SettingsSections() []web.SettingsSectionDefinition {
	var defs []web.SettingsSectionDefinition
	for _, s := range m.sections {
		defs = append(defs, s.Definition())
	}
	return defs
}

func (m *mockWebService) SettingsSection(id string) (web.SettingsSection, bool) {
	for _, s := range m.sections {
		if s.Definition().ID == id {
			return s, true
		}
	}
	return nil, false
}

func TestPluginRegistersAndHandlesForm(t *testing.T) {
	h := host.NewHost()
	mockWeb := newMockWebService()
	if err := h.RegisterService("web", web.Service(mockWeb)); err != nil {
		t.Fatal(err)
	}

	p := New()
	if err := p.Start(h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })

	if len(mockWeb.sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(mockWeb.sections))
	}
	def := mockWeb.sections[0].Definition()
	if def.ID != "demo" || def.Title != "演示设置" || def.Order != 10 {
		t.Fatalf("unexpected definition: %#v", def)
	}

	component, err := mockWeb.sections[0].Render()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := component.Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	rendered := sb.String()
	if !strings.Contains(rendered, "演示插件设置") || !strings.Contains(rendered, "Explorer") {
		t.Fatalf("unexpected render: %s", rendered)
	}

	handler, ok := mockWeb.routes["POST /settings/demo"]
	if !ok {
		t.Fatal("route POST /settings/demo not registered")
	}

	form := url.Values{"nickname": {"Alice"}}
	req := httptest.NewRequest(nethttp.MethodPost, "/settings/demo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "Alice") || !strings.Contains(respBody, "设置已保存") {
		t.Fatalf("unexpected response after save: %s", respBody)
	}
	_ = io.Discard
}
