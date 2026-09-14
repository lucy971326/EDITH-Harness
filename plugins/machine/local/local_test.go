package machinelocal

import (
	"os"
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

func TestLocal_homeDirAndReadDir(t *testing.T) {
	m := newTestLocal(t)
	home, err := m.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home == "" || !filepath.IsAbs(home) {
		t.Fatalf("HomeDir() = %q", home)
	}

	dir := t.TempDir()
	err = os.WriteFile(filepath.Join(dir, "note.txt"), []byte("note"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := m.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "note.txt" || entries[0].IsDir || !entries[0].IsFile {
		t.Fatalf("ReadDir() = %#v", entries)
	}
}

func TestLocal_resolvePath(t *testing.T) {
	m := newTestLocal(t)
	if got := m.ResolvePath("/work", "nested/note.txt"); got != filepath.Join("/work", "nested", "note.txt") {
		t.Fatalf("ResolvePath(relative) = %q", got)
	}
	absolute := filepath.Join(t.TempDir(), "note.txt")
	if got := m.ResolvePath("/work", absolute); got != filepath.Clean(absolute) {
		t.Fatalf("ResolvePath(absolute) = %q", got)
	}
}

func newTestLocal(t *testing.T) *local {
	t.Helper()
	m, err := newLocal()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.close(); err != nil {
			t.Error(err)
		}
	})
	return m
}
