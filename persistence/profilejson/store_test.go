package profilejson

import (
	"os"
	"testing"

	"harness/agents"
)

func TestStorePersistsProfileAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	profile := agents.AgentProfile{
		ID:           "小红",
		Revision:     1,
		Provider:     "deepseek",
		Model:        "test-model",
		Thinking:     "off",
		SystemPrompt: "你是客服",
		Tools:        []string{"echo"},
	}
	err = first.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(t, root, 0o700)
	assertPrivate(t, first.path("小红"), 0o600)

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := second.Get("小红")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "小红" || loaded.Revision != 1 || loaded.Model != "test-model" || len(loaded.Tools) != 1 {
		t.Fatalf("重启后档案不对：%+v", loaded)
	}

	loaded.Model = "new-model"
	loaded.Revision++
	err = second.Update(loaded)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := second.Get("小红")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Model != "new-model" {
		t.Fatalf("更新后档案不对：%+v", updated)
	}

	err = second.Archive("小红")
	if err != nil {
		t.Fatal(err)
	}
	archived, err := second.Get("小红")
	if err != nil {
		t.Fatal(err)
	}
	if !archived.Archived || archived.Revision != 3 {
		t.Fatalf("归档后档案不对：%+v", archived)
	}
}

func TestStoreReadsOldProfileWithoutModelOwnership(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(store.path("老小红"), []byte(`{"ID":"老小红","Revision":1}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get("老小红")
	if err != nil {
		t.Fatalf("旧档案应仍能读取：%v", err)
	}
	if loaded.Provider != "" || loaded.Model != "" || loaded.Thinking != "" {
		t.Fatalf("旧档案字段应保持为空：%+v", loaded)
	}
}

func assertPrivate(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s 权限是 %o，想要 %o", path, info.Mode().Perm(), want)
	}
}
