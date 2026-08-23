package session

import (
	"strings"
	"testing"

	"harness/core"
)

func TestPluginRegistersStore(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	journal := NewMemoryJournal()
	app.RegisterService("journal", journal)
	err := app.Install(Plugin{})
	if err != nil {
		t.Fatalf("session 插件启动失败：%v", err)
	}

	store, err := Get(app)
	if err != nil {
		t.Fatalf("取不到 session Store：%v", err)
	}
	if store == nil {
		t.Fatal("session 插件登记的 Store 不该是 nil")
	}

	_, err = store.Create("测试账", "测试 Agent", 1)
	if err != nil {
		t.Fatalf("登记的 Journal 没有生效：%v", err)
	}
}

func TestPluginRequiresJournal(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(Plugin{})
	if err == nil {
		t.Fatal("没有 Journal 时 session 插件应该启动失败")
	}
	if !strings.Contains(err.Error(), "Journal") {
		t.Fatalf("错误应该说明缺少 Journal：%v", err)
	}
}
