package chat

import (
	"testing"

	"github.com/a-h/templ"
)

func TestDockRegistryRegistersAndSortsDefinitions(t *testing.T) {
	registry := newDockRegistry()
	for _, definition := range []DockDefinition{
		{ID: "second", Name: "第二", Order: 20},
		{ID: "first", Name: "第一", Order: 10},
		{ID: "alpha", Name: "甲", Order: 10},
	} {
		if err := registry.RegisterDock(testDock{definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	docks := registry.Docks()
	if len(docks) != 3 || docks[0].ID != "alpha" || docks[1].ID != "first" || docks[2].ID != "second" {
		t.Fatalf("docks = %#v", docks)
	}
	if _, ok := registry.Dock("first"); !ok {
		t.Fatal("first dock is missing")
	}
}

func TestDockRegistryRejectsInvalidAndDuplicateDocks(t *testing.T) {
	registry := newDockRegistry()
	if err := registry.RegisterDock(nil); err == nil {
		t.Fatal("RegisterDock(nil) succeeded")
	}
	for _, definition := range []DockDefinition{
		{ID: "", Name: "empty"},
		{ID: "Upper", Name: "uppercase"},
		{ID: "7starts", Name: "number"},
		{ID: "valid", Name: "  "},
	} {
		if err := registry.RegisterDock(testDock{definition: definition}); err == nil {
			t.Fatalf("RegisterDock(%#v) succeeded", definition)
		}
	}
	valid := testDock{definition: DockDefinition{ID: "demo", Name: "演示"}}
	if err := registry.RegisterDock(valid); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterDock(valid); err == nil {
		t.Fatal("RegisterDock accepted duplicate")
	}
}

type testDock struct {
	definition DockDefinition
}

func (d testDock) Definition() DockDefinition {
	return d.definition
}

func (testDock) Render(DockContext) (templ.Component, error) {
	return templ.Raw("ready"), nil
}
