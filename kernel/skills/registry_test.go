package skills

import "testing"

func TestRegistry_getPreservesSelectionOrder(t *testing.T) {
	registry := NewRegistry()
	for _, skill := range []Skill{
		{Name: "git", Summary: "Git workflow."},
		{Name: "go", Summary: "Go workflow."},
	} {
		err := registry.Register(skill)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := registry.Get([]string{"go", "git"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "go" || got[1].Name != "git" {
		t.Fatalf("Get() = %#v", got)
	}
	list := registry.List()
	if list[0].Name != "git" || list[1].Name != "go" {
		t.Fatalf("List() = %#v", list)
	}
}

func TestRegistry_rejectsInvalidDuplicateAndMissingSkills(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Skill{})
	if err == nil {
		t.Fatal("Register(empty) error = nil")
	}
	skill := Skill{Name: "git", Summary: "Git workflow."}
	err = registry.Register(skill)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(skill)
	if err == nil {
		t.Fatal("duplicate Register() error = nil")
	}
	if _, err := registry.Get([]string{"missing"}); err == nil {
		t.Fatal("Get(missing) error = nil")
	}
	if _, err := registry.Get([]string{"git", "git"}); err == nil {
		t.Fatal("Get(duplicate) error = nil")
	}
}
