package chat

import "testing"

func TestSuggestionRegistryMergesSourcesByStableIDAndRejectsDuplicateID(t *testing.T) {
	registry := newSuggestionRegistry()
	err := registry.Register(testSuggestionSource{id: "skills", prefixes: []string{"/", "$"}, items: []Suggestion{
		{Name: "second"}, {Name: "first"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(testSuggestionSource{id: "commands", prefixes: []string{"/"}, items: []Suggestion{{Name: "command"}}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := registry.List("/", SuggestionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Name != "command" || items[0].SourceID != "commands" ||
		items[1].Name != "first" || items[1].SourceID != "skills" ||
		items[2].Name != "second" || items[2].SourceID != "skills" {
		t.Fatalf("items = %#v", items)
	}
	dollarItems, err := registry.List("$", SuggestionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(dollarItems) != 2 || dollarItems[0].SourceID != "skills" || dollarItems[1].SourceID != "skills" {
		t.Fatalf("dollar items = %#v", dollarItems)
	}
	if err := registry.Register(testSuggestionSource{id: "skills", prefixes: []string{"@"}}); err == nil {
		t.Fatal("duplicate source ID was accepted")
	}
}

type testSuggestionSource struct {
	id       string
	prefixes []string
	items    []Suggestion
}

func (s testSuggestionSource) ID() string { return s.id }

func (s testSuggestionSource) Prefixes() []string { return s.prefixes }

func (s testSuggestionSource) List(SuggestionContext) ([]Suggestion, error) {
	return append([]Suggestion(nil), s.items...), nil
}
