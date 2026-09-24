package settings

import (
	"os"
	"testing"

	"harness/kernel/permissions"
	"harness/kernel/persist"
)

func TestSessionSettings_putFor(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	s := NewStore(files)
	if err != nil {
		t.Fatal(err)
	}

	in := SessionSettings{
		AgentID:         "default",
		Model:           "deepseek-v4",
		ReasoningEffort: "high",
		Workspace:       "/workspace",
		PermissionMode:  permissions.ReadOnly,
	}
	err = s.Put("chat1", in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.For("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("got %+v", got)
	}
	scope, err := s.sessionFiles("chat1")
	if err != nil {
		t.Fatal(err)
	}
	path, err := scope.Path("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session settings file: %v", err)
	}
	in.PermissionMode = "unknown"
	if err = s.Put("chat1", in); err == nil {
		t.Fatal("unknown mode saved")
	}
}

func TestSessionSettings_UsesAgent(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	s := NewStore(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("chat1", SessionSettings{AgentID: "coding"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("chat2", SessionSettings{AgentID: "default"}); err != nil {
		t.Fatal(err)
	}
	used, err := s.UsesAgent("coding")
	if err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("UsesAgent(coding) = false")
	}
	used, err = s.UsesAgent("missing")
	if err != nil {
		t.Fatal(err)
	}
	if used {
		t.Fatal("UsesAgent(missing) = true")
	}
}
