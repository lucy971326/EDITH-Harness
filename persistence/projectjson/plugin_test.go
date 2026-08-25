package projectjson

import (
	"testing"

	"harness/core"
	"harness/projects"
)

func TestPluginRegistersProjectStore(t *testing.T) {
	app := core.New()
	err := (Plugin{Root: t.TempDir()}).Start(app)
	if err != nil {
		t.Fatal(err)
	}
	store, err := core.Resolve[projects.Store](app, "project-store")
	if err != nil {
		t.Fatal(err)
	}
	if store == nil {
		t.Fatal("项目存储没有登记")
	}
}
