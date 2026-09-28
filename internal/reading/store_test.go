package reading

import (
	"harness/internal/persist"
	"os"
	"sync"
	"testing"
)

func TestPositionIsMonotonicAndDurable(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for seq := uint64(1); seq <= 24; seq++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := store.Advance("session", seq); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	restarted, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Advance("session", 2); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Position("session")
	if err != nil || got != 24 {
		t.Fatalf("position = %d, %v", got, err)
	}
}

func TestFailedPersistenceDoesNotBecomeRead(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	// 目录被普通文件占用：跨平台触发持久化失败，不依赖用户权限。
	if err = files.Write("reading", []byte("blocked")); err != nil {
		t.Fatal(err)
	}
	if err = store.Advance("session", 12); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	if err = files.Remove("reading"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Position("session")
	if err != nil || got != 0 {
		t.Fatalf("failed write became read: %d, %v", got, err)
	}
	dir, err := files.Scope("reading")
	if err != nil {
		t.Fatal(err)
	}
	if err = dir.Write("session.json", []byte("broken")); err != nil {
		t.Fatal(err)
	}
	if err = store.Advance("session", 12); err == nil {
		t.Fatal("corrupt position silently overwritten")
	}
	data, err := dir.Read("session.json")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if string(data) != "broken" {
		t.Fatalf("corrupt record replaced: %q", data)
	}
}
