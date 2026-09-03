package chat

import (
	"testing"

	"github.com/a-h/templ"
)

func TestPanelRegistryRegistersAndSortsDefinitions(t *testing.T) {
	registry := newPanelRegistry()
	err := registry.RegisterPanel(testPanel{definition: PanelDefinition{
		ID: "second", Name: "第二", Icon: "2", Order: 20, DefaultInstanceKey: "main", DefaultTabTitle: "第二",
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterPanel(testPanel{definition: PanelDefinition{
		ID: "first", Name: "第一", Icon: "1", Order: 10, DefaultInstanceKey: "main", DefaultTabTitle: "第一",
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterPanel(testPanel{definition: PanelDefinition{
		ID: "alpha", Name: "甲", Icon: "A", Order: 10, DefaultInstanceKey: "main", DefaultTabTitle: "甲",
	}})
	if err != nil {
		t.Fatal(err)
	}
	panels := registry.Panels()
	if len(panels) != 3 || panels[0].ID != "alpha" || panels[1].ID != "first" || panels[2].ID != "second" {
		t.Fatalf("panels = %#v", panels)
	}
	if _, ok := registry.Panel("first"); !ok {
		t.Fatal("first panel is missing")
	}
}

func TestPanelRegistryRejectsInvalidAndDuplicatePanels(t *testing.T) {
	registry := newPanelRegistry()
	if err := registry.RegisterPanel(nil); err == nil {
		t.Fatal("RegisterPanel(nil) succeeded")
	}
	invalid := testPanel{definition: PanelDefinition{ID: "invalid"}}
	if err := registry.RegisterPanel(invalid); err == nil {
		t.Fatal("RegisterPanel(invalid) succeeded")
	}
	valid := testPanel{definition: PanelDefinition{
		ID: "demo", Name: "演示", Icon: "D", DefaultInstanceKey: "main", DefaultTabTitle: "演示",
	}}
	err := registry.RegisterPanel(valid)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterPanel(valid)
	if err == nil {
		t.Fatal("RegisterPanel accepted duplicate")
	}
}

type testPanel struct {
	definition PanelDefinition
}

func (p testPanel) Definition() PanelDefinition {
	return p.definition
}

func (testPanel) Render(PanelContext, PanelTab) (templ.Component, error) {
	return templ.Raw("ready"), nil
}
