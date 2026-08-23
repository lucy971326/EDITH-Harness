package profilejson

import (
	"testing"

	"harness/loop"
)

func TestStorePersistsProfileAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	profile := loop.AgentProfile{
		ID:           "小红",
		Revision:     1,
		Model:        "test-model",
		SystemPrompt: "你是客服",
		Tools:        []string{"echo"},
	}
	err = first.Create(profile)
	if err != nil {
		t.Fatal(err)
	}

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
