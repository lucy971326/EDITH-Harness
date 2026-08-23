package jsonl

import (
	"testing"

	"harness/core"
	"harness/session"
)

func TestPluginProvidesJournalForSession(t *testing.T) {
	root := t.TempDir()
	firstApp := core.New()

	err := firstApp.Install(
		Plugin{Root: root},
		session.Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Get(firstApp)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.Create("插件账", "测试 Agent", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordUserMessage("自动落盘")
	if err != nil {
		t.Fatal(err)
	}
	firstApp.Close()

	secondApp := core.New()
	defer secondApp.Close()
	err = secondApp.Install(
		Plugin{Root: root},
		session.Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := session.Get(secondApp)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := secondStore.Open("插件账")
	if err != nil {
		t.Fatal(err)
	}
	history := opened.ModelHistory()
	if len(history) != 1 || history[0].Text != "自动落盘" {
		t.Fatalf("重启后应该读回 JSONL 旧账：%+v", history)
	}
}
