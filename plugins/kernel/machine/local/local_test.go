package machinelocal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"harness/kernel/host"
	"harness/kernel/machine"
)

func TestPluginStart_registersMachine(t *testing.T) {
	h := host.NewHost()
	t.Cleanup(func() {
		_ = h.Close()
	})

	err := h.Install(New())
	if err != nil {
		t.Fatal(err)
	}

	_, err = host.Resolve[machine.Machine](h, "machine")
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocal_writeThenRead(t *testing.T) {
	m := newTestLocal(t)
	path := filepath.Join(t.TempDir(), "nested", "note.txt")

	err := m.WriteFile(path, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	err = m.WriteFile(path, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := m.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("ReadFile() = %q, want second", got)
	}
}

func TestLocal_run(t *testing.T) {
	m := newTestLocal(t)
	dir := t.TempDir()

	stdout, stderr, err := m.Run(context.Background(), dir, []string{"bash", "-c", "test -d . && printf out && printf err >&2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "out" {
		t.Errorf("stdout = %q, want out", stdout)
	}
	if string(stderr) != "err" {
		t.Errorf("stderr = %q, want err", stderr)
	}
}

func TestLocal_runFailureKeepsOutput(t *testing.T) {
	m := newTestLocal(t)

	stdout, stderr, err := m.Run(context.Background(), t.TempDir(), []string{"bash", "-c", "printf out && printf err >&2 && exit 7"})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if string(stdout) != "out" {
		t.Errorf("stdout = %q, want out", stdout)
	}
	if string(stderr) != "err" {
		t.Errorf("stderr = %q, want err", stderr)
	}
}

func TestLocal_runRejectsEmptyArgv(t *testing.T) {
	m := newTestLocal(t)

	_, _, err := m.Run(context.Background(), t.TempDir(), nil)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
}

func TestLocal_runHonorsCanceledContext(t *testing.T) {
	m := newTestLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := m.Run(ctx, t.TempDir(), []string{"bash", "-c", "sleep 1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func newTestLocal(t *testing.T) *local {
	t.Helper()
	m, err := newLocal()
	if err != nil {
		t.Fatal(err)
	}
	return m
}
