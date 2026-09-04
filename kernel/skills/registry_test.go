package skills

import "testing"

func TestRegistry_listSortsByName(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(testProvider{name: "one", skills: []Skill{
		{Name: "git", Description: "Git workflow.", Location: "/skills/git", Scope: ScopeUser},
		{Name: "go", Description: "Go workflow.", Location: "/skills/go", Scope: ScopeUser},
	}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := registry.List("/work")
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Name != "git" || list[1].Name != "go" {
		t.Fatalf("List() = %#v", list)
	}
}

func TestRegistry_rejectsInvalidDuplicateAndMissingSkills(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(testProvider{name: "invalid", skills: []Skill{{}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.List("/work"); err == nil {
		t.Fatal("List(empty) error = nil")
	}
	registry = NewRegistry()
	skill := Skill{Name: "git", Description: "Git workflow.", Location: "/skills/git", Scope: ScopeUser}
	err = registry.Register(testProvider{name: "one", skills: []Skill{skill}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(testProvider{name: "one", skills: []Skill{skill}})
	if err == nil {
		t.Fatal("Register(duplicate provider) error = nil")
	}
}

func TestRegistry_rejectsDuplicateNamesAcrossProviders(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(testProvider{name: "one", skills: []Skill{{Name: "same", Description: "one", Location: "/one", Scope: ScopeUser}}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(testProvider{name: "two", skills: []Skill{{Name: "same", Description: "two", Location: "/two", Scope: ScopeUser}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.List("/work")
	if err == nil {
		t.Fatal("List() error = nil")
	}
}

type testProvider struct {
	name   string
	skills []Skill
}

func (p testProvider) Name() string { return p.name }

func (p testProvider) List(string) ([]Skill, error) {
	return append([]Skill(nil), p.skills...), nil
}
