package persist

import (
	"testing"
)

func TestFilesScopeAppendAndRejectTraversal(t *testing.T) {
	files, err := NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	module, err := files.Scope("module")
	if err != nil {
		t.Fatal(err)
	}
	if err := module.Write("state.jsonl", []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := module.Append("state.jsonl", []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	data, err := module.Read("state.jsonl")
	if err != nil || string(data) != "one\ntwo\n" {
		t.Fatalf("Read() = %q, %v", data, err)
	}
	if _, err := files.Scope(".."); err == nil {
		t.Fatal("Scope accepted traversal")
	}
}

func TestLockRootExclusiveAndReleased(t *testing.T) {
	root := t.TempDir()
	first, err := LockRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockRoot(root); err == nil {
		second.Close()
		first.Close()
		t.Fatal("second backend acquired the same data directory")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := LockRoot(root)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	defer second.Close()
}
