package jsonl

import (
	"testing"

	"harness/core"
	"harness/session"
)

func TestPluginProvidesJournalForSession(t *testing.T) {
	app := core.New()
	defer app.Close()

	err := app.Install(
		Plugin{Root: t.TempDir()},
		session.Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.Create("插件账")
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordUserMessage("自动落盘")
	if err != nil {
		t.Fatal(err)
	}
}
