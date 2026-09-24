package hooks_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"harness/internal/hooks"
	"harness/internal/machine"
	machinelocal "harness/internal/machine/local"
	"harness/internal/persist"
	"harness/internal/tools"
)

func fixture(t *testing.T) (*hooks.Service, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test commands use /bin/sh")
	}
	filesystem, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filesystem.Close() })

	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := hooks.NewService(files, filesystem)
	if err != nil {
		t.Fatal(err)
	}
	return service, t.TempDir()
}

func shell(name, code string) hooks.Hook {
	return hooks.Hook{Name: name, Enabled: true, Command: "/bin/sh", Args: []string{"-c", code}}
}

func TestCheckRunsInOrderAndBlocksOnlyOnDeny(t *testing.T) {
	service, workspace := fixture(t)
	marker := filepath.Join(workspace, "calls.log")
	_, err := service.Save(hooks.SaveInput{Scope: "global", Hooks: []hooks.Hook{
		shell("first", "printf first >> '"+marker+"'"),
		shell("failed", "exit 7"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	project := shell("deny", "printf '{\"decision\":\"deny\",\"reason\":\"blocked\"}'")
	_, err = service.Save(hooks.SaveInput{Scope: "project", Workspace: workspace, Hooks: []hooks.Hook{project}})
	if err != nil {
		t.Fatal(err)
	}
	var notices []string
	call := tools.Call{Name: "read", Arguments: json.RawMessage(`{"path":"a"}`), Workspace: workspace, ToolCallID: "t1"}
	reason, err := service.Check(context.Background(), call, func(message string) { notices = append(notices, message) })
	if err != nil || reason != "blocked" {
		t.Fatalf("Check() = %q, %v", reason, err)
	}
	body, err := os.ReadFile(marker)
	if err != nil || string(body) != "first" {
		t.Fatalf("order marker = %q, %v", body, err)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "failed") {
		t.Fatalf("notices = %v", notices)
	}
}

func TestProjectConfigChangeRequiresTrustAndSaveChecksVersion(t *testing.T) {
	service, workspace := fixture(t)
	first := shell("project", "printf '{\"decision\":\"deny\",\"reason\":\"one\"}'")
	view, err := service.Save(hooks.SaveInput{Scope: "project", Workspace: workspace, Hooks: []hooks.Hook{first}})
	if err != nil || !view.Trusted {
		t.Fatalf("Save() = %#v, %v", view, err)
	}
	second := shell("project", "printf '{\"decision\":\"deny\",\"reason\":\"two\"}'")
	body, err := json.Marshal(struct {
		Hooks []hooks.Hook `json:"hooks"`
	}{[]hooks.Hook{second}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".harness", "hooks.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	view, err = service.View(workspace)
	if err != nil || view.Trusted {
		t.Fatalf("external change view = %#v, %v", view, err)
	}
	_, err = service.Save(hooks.SaveInput{Scope: "project", Workspace: workspace, Hash: "stale", Hooks: []hooks.Hook{first}})
	if !errors.Is(err, machine.ErrFileConflict) {
		t.Fatalf("stale save error = %v", err)
	}
	var notice string
	call := tools.Call{Name: "read", Arguments: json.RawMessage(`{}`), Workspace: workspace}
	reason, err := service.Check(context.Background(), call, func(message string) { notice = message })
	if err != nil || reason != "" || !strings.Contains(notice, "信任") && !strings.Contains(notice, "确认") {
		t.Fatalf("untrusted Check() = %q, %v, %q", reason, err, notice)
	}
	_, err = service.Trust(hooks.TrustInput{Workspace: workspace, Hash: view.Project.Hash})
	if err != nil {
		t.Fatal(err)
	}
	reason, err = service.Check(context.Background(), call, nil)
	if err != nil || reason != "two" {
		t.Fatalf("trusted Check() = %q, %v", reason, err)
	}
}

func TestHookCancellationStopsTool(t *testing.T) {
	service, workspace := fixture(t)
	started := filepath.Join(workspace, "child-started")
	leaked := filepath.Join(workspace, "child-leaked")
	command := "(printf started > '" + started + "'; sleep 0.5; printf leaked > '" + leaked + "') & wait"
	_, err := service.Save(hooks.SaveInput{Scope: "global", Hooks: []hooks.Hook{shell("wait", command)}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, checkErr := service.Check(ctx, tools.Call{Name: "read", Arguments: json.RawMessage(`{}`), Workspace: workspace}, nil)
		done <- checkErr
	}()
	deadline := time.After(2 * time.Second)
	for {
		if _, statErr := os.Stat(started); statErr == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Hook child did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Check() error = %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(leaked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Hook child continued after cancellation: %v", err)
	}
}

func TestInvalidOutputReportsAndContinues(t *testing.T) {
	service, workspace := fixture(t)
	_, err := service.Save(hooks.SaveInput{Scope: "global", Hooks: []hooks.Hook{shell("bad-json", "printf 'hello'")}})
	if err != nil {
		t.Fatal(err)
	}
	var notice string
	reason, err := service.Check(context.Background(), tools.Call{
		Name: "read", Arguments: json.RawMessage(`{}`), Workspace: workspace,
	}, func(message string) { notice = message })
	if err != nil || reason != "" || !strings.Contains(notice, "invalid stdout JSON") {
		t.Fatalf("Check() = %q, %v; notice = %q", reason, err, notice)
	}
}
