package commands

import (
	"context"
	"strings"
	"testing"
)

type testCommand struct {
	name        string
	description string
	sessionID   string
}

func (c *testCommand) Name() string        { return c.name }
func (c *testCommand) Description() string { return c.description }
func (c *testCommand) Run(_ context.Context, sessionID string) error {
	c.sessionID = sessionID
	return nil
}

func TestRegistryRegisterListAndRun(t *testing.T) {
	registry := NewRegistry()
	compact := &testCommand{name: "compact", description: "compress history"}
	err := registry.Register(compact)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(&testCommand{name: "compact", description: "again"})
	if err == nil {
		t.Fatal("duplicate command was accepted")
	}
	list := registry.List()
	if len(list) != 1 || list[0].Name != "compact" {
		t.Fatalf("list = %#v", list)
	}
	command, err := registry.Get("compact")
	if err != nil {
		t.Fatal(err)
	}
	err = command.Run(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if compact.sessionID != "session-1" {
		t.Fatalf("sessionID = %q", compact.sessionID)
	}
}

func TestGetRejectsUnknown(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Get("compact")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unknown command error = %v", err)
	}
}
