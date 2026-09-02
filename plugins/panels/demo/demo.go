package demo

import (
	"fmt"

	"github.com/a-h/templ"

	"harness/plugins/products/chat"
)

type panel struct{}

func (panel) Definition() chat.PanelDefinition {
	return chat.PanelDefinition{
		ID:                 "demo",
		Name:               "演示面板",
		Icon:               "▤",
		Order:              10,
		DefaultInstanceKey: "main",
		DefaultTabTitle:    "演示面板",
	}
}

func (panel) Render(_ chat.PanelContext, tab chat.PanelTab) (templ.Component, error) {
	if tab.InstanceKey != "main" {
		return nil, fmt.Errorf("demo: unknown instance %q", tab.InstanceKey)
	}
	return content(), nil
}
