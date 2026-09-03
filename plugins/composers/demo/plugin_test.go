package demo

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/plugins/products/chat"
)

func TestPluginRegistersDemoComposerAction(t *testing.T) {
	h := host.NewHost()
	service := &mockChatService{}
	err := h.RegisterService("chat", chat.Service(service))
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(New())
	if err != nil {
		t.Fatal(err)
	}
	if service.action == nil || service.action.Definition().ID != "demo" {
		t.Fatalf("action = %#v", service.action)
	}

	component, err := service.action.Render(chat.ComposerActionContext{
		SessionID: "sess-1",
		Workspace: "/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "⚡ 模版") {
		t.Fatalf("rendered output missing template button: %s", html)
	}
	if !strings.Contains(html, "审查代码") || !strings.Contains(html, "type=\"button\"") {
		t.Fatalf("rendered output missing buttons: %s", html)
	}
	// Action 不得嵌套 <form>
	if strings.Contains(html, "<form") {
		t.Fatalf("action must not contain nested <form>: %s", html)
	}
}

type mockChatService struct {
	action chat.ComposerAction
}

func (*mockChatService) RegisterPanel(chat.Panel) error { return nil }
func (*mockChatService) Panels() []chat.PanelDefinition  { return nil }
func (*mockChatService) Panel(string) (chat.Panel, bool) { return nil, false }

func (*mockChatService) RegisterDock(chat.Dock) error { return nil }
func (*mockChatService) Docks() []chat.DockDefinition { return nil }
func (*mockChatService) Dock(string) (chat.Dock, bool) { return nil, false }

func (*mockChatService) RegisterMessageAction(chat.MessageAction) error { return nil }
func (*mockChatService) MessageActions() []chat.MessageActionDefinition { return nil }
func (*mockChatService) MessageAction(string) (chat.MessageAction, bool) { return nil, false }

func (s *mockChatService) RegisterComposerAction(action chat.ComposerAction) error {
	s.action = action
	return nil
}

func (*mockChatService) ComposerActions() []chat.ComposerActionDefinition { return nil }
func (*mockChatService) ComposerAction(string) (chat.ComposerAction, bool) { return nil, false }
