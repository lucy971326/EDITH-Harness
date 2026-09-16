package subagents

import (
	"errors"
	"testing"
)

func TestRecoveredTaskGraphDepthAndCycles(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	validStore, err := newTaskStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []TaskRecord{
		{Version: 1, ID: "task-child", ParentSessionID: "root", ChildSessionID: "child", Description: "child"},
		{Version: 1, ID: "task-grandchild", ParentSessionID: "child", ChildSessionID: "grandchild", Description: "grandchild"},
	} {
		if err := validStore.saveTask(record); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := newSubagentsWithStore(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, validStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}

	if err := validStore.saveTask(TaskRecord{
		Version: 1, ID: "task-too-deep", ParentSessionID: "grandchild", ChildSessionID: "great-grandchild", Description: "too deep",
	}); err != nil {
		t.Fatal(err)
	}
	_, err = newSubagentsWithStore(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, validStore)
	if !errors.Is(err, ErrDepthLimit) {
		t.Fatalf("expected recovered depth limit error, got %v", err)
	}

	cycleStore, err := newTaskStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []TaskRecord{
		{Version: 1, ID: "task-a", ParentSessionID: "session-b", ChildSessionID: "session-a", Description: "a"},
		{Version: 1, ID: "task-b", ParentSessionID: "session-a", ChildSessionID: "session-b", Description: "b"},
	} {
		if err := cycleStore.saveTask(record); err != nil {
			t.Fatal(err)
		}
	}
	_, err = newSubagentsWithStore(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, cycleStore)
	if !errors.Is(err, ErrInvalidTaskData) {
		t.Fatalf("expected recovered cycle error, got %v", err)
	}
}
