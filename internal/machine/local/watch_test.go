package machinelocal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"harness/internal/machine"
)

func TestLocal_watchFileAcrossDeleteAndRecreate(t *testing.T) {
	m := newTestLocal(t)
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	watch, err := m.Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })

	if err = os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForChangedPath(t, watch, path)
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("recreated"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForChangedPath(t, watch, path)
}

func TestLocal_watchFileSymlinkReportsRequestedPath(t *testing.T) {
	m := newTestLocal(t)
	root := t.TempDir()
	targetDir := filepath.Join(root, "target")
	linkDir := filepath.Join(root, "link")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "note.txt")
	link := filepath.Join(linkDir, "note.txt")
	if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	watch, err := m.Watch(link)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })
	if err = os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForChangedPath(t, watch, link)
}

func TestLocal_watchDirectoryIsNotRecursive(t *testing.T) {
	m := newTestLocal(t)
	dir := t.TempDir()
	watch, err := m.Watch(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })

	nested := filepath.Join(dir, "nested")
	if err = os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	waitForChangedPath(t, watch, nested)
	if err = os.WriteFile(filepath.Join(nested, "hidden.txt"), []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-watch.Events():
		t.Fatalf("recursive event = %#v", event)
	case <-time.After(2 * watchDebounce):
	}
}

func waitForChangedPath(t *testing.T, watch machine.FileWatch, path string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-watch.Events():
			if event.Error != nil {
				t.Fatal(event.Error)
			}
			for _, changed := range event.ChangedPaths {
				if samePath(changed, path) {
					return
				}
			}
		case <-deadline:
			t.Fatalf("no event for %q", path)
		}
	}
}
