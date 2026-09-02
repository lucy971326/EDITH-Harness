package demo

import (
	"testing"

	"harness/kernel/host"
	"harness/plugins/products/chat"
)

func TestPluginRegistersDemoPanel(t *testing.T) {
	h := host.NewHost()
	service := &chatService{}
	err := h.RegisterService("chat", chat.Service(service))
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(New())
	if err != nil {
		t.Fatal(err)
	}
	if service.panel == nil || service.panel.Definition().ID != "demo" {
		t.Fatalf("panel = %#v", service.panel)
	}
}

type chatService struct {
	panel chat.Panel
}

func (s *chatService) RegisterPanel(panel chat.Panel) error {
	s.panel = panel
	return nil
}

func (*chatService) Panels() []chat.PanelDefinition {
	return nil
}

func (*chatService) Panel(string) (chat.Panel, bool) {
	return nil, false
}

func (*chatService) RegisterMessageAction(chat.MessageAction) error {
	return nil
}

func (*chatService) MessageActions() []chat.MessageActionDefinition {
	return nil
}

func (*chatService) MessageAction(string) (chat.MessageAction, bool) {
	return nil, false
}
