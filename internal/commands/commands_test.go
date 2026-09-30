package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/internal/machine"
	machinelocal "harness/internal/machine/local"
	"harness/internal/persist"
)

type testCommand struct {
	name        string
	description string
	sessionID   string
}

// 同名命令必须保留来源身份；旧版本保存不能覆盖另一处改动。
func TestPromptStoreScopesAndConflict(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	store, err := NewPromptStore(files, local)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	user, err := store.Save("user", "", "", Prompt{Name: "review", Prompt: "用户 $ARGUMENTS"}, true)
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Save("workspace", workspace, "", Prompt{Name: "review", Prompt: "项目 $ARGUMENTS"}, true)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.List(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "workspace:review" || items[1].ID != "user:review" {
		t.Fatalf("commands = %#v", items)
	}
	selected, err := store.Resolve(workspace, "workspace:review")
	if err != nil || selected.Prompt != "项目 $ARGUMENTS" {
		t.Fatalf("resolve = %#v, %v", selected, err)
	}
	_, err = store.Save("user", "", user.Hash, Prompt{Name: "review", Prompt: "重复"}, true)
	if err == nil {
		t.Fatal("duplicate create was accepted")
	}
	_, err = store.Save("workspace", workspace, "stale", Prompt{Name: "review", Prompt: "覆盖"}, false)
	if !errors.Is(err, machine.ErrFileConflict) {
		t.Fatalf("stale save = %v", err)
	}
	_, err = store.Delete("workspace", workspace, project.Hash, "review")
	if err != nil {
		t.Fatal(err)
	}
	items, err = store.List(workspace)
	if err != nil || len(items) != 1 || items[0].ID != "user:review" {
		t.Fatalf("after delete = %#v, %v", items, err)
	}
	path := filepath.Join(workspace, ".harness", "commands.json")
	if err = os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Read("workspace", workspace); err == nil {
		t.Fatal("corrupt configuration was accepted")
	}
}

func TestPromptExpansion(t *testing.T) {
	item := Prompt{Name: "review", Prompt: "检查 $ARGUMENTS"}
	text, err := Expand(item, "/review src")
	if err != nil || text != "检查 src" {
		t.Fatalf("expanded = %q, %v", text, err)
	}
	item.Prompt = "检查代码"
	text, err = Expand(item, "/review src")
	if err != nil || text != "检查代码\n\nsrc" {
		t.Fatalf("appended = %q, %v", text, err)
	}
	if _, err = Expand(item, "/reviewer"); err == nil {
		t.Fatal("wrong invocation was accepted")
	}
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
