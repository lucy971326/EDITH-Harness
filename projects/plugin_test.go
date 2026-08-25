package projects

import (
	"strings"
	"testing"

	"harness/core"
	"harness/session"
)

func TestPluginRegistersService(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	app.RegisterService("project-store", Store(newMemoryStore()))
	app.RegisterService("sessions", session.NewStore(session.NewMemoryJournal(), app))

	err := app.Install(Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := Get(app)
	if err != nil {
		t.Fatal(err)
	}
	if service == nil {
		t.Fatal("项目服务没有登记")
	}
}

func TestPluginRequiresDependencies(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(Plugin{})
	if err == nil {
		t.Fatal("缺少依赖时项目插件应启动失败")
	}
	if !strings.Contains(err.Error(), "项目存储") {
		t.Fatalf("错误没有说明缺少项目存储：%v", err)
	}
}
