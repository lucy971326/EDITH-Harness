package loops

import (
	"context"
	"testing"
)

func TestRegistry_registerGetAndList(t *testing.T) {
	registry := NewRegistry()
	loop := testLoop{definition: Definition{Kind: "react", Description: "React loop."}}
	err := registry.Register(loop)
	if err != nil {
		t.Fatal(err)
	}
	got, err := registry.Get("react")
	if err != nil {
		t.Fatal(err)
	}
	if got.Definition() != loop.definition {
		t.Fatalf("definition = %#v", got.Definition())
	}
	if definitions := registry.Definitions(); len(definitions) != 1 || definitions[0] != loop.definition {
		t.Fatalf("definitions = %#v", definitions)
	}
}

func TestRegistry_rejectsInvalidAndDuplicateLoops(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(nil)
	if err == nil {
		t.Fatal("Register(nil) error = nil")
	}
	err = registry.Register(testLoop{})
	if err == nil {
		t.Fatal("Register(empty) error = nil")
	}
	loop := testLoop{definition: Definition{Kind: "react", Description: "React loop."}}
	err = registry.Register(loop)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(loop)
	if err == nil {
		t.Fatal("duplicate Register() error = nil")
	}
}

type testLoop struct {
	definition Definition
}

func (l testLoop) Definition() Definition { return l.definition }

func (testLoop) Run(context.Context, Invocation) error { return nil }
