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
