package presetjson

import (
	"os"
	"testing"

	"harness/core"
	"harness/presets"
)

func TestStorePersistsAllRevisionsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	initial := presets.Revision{
		ID:           "小红",
		Revision:     1,
		SystemPrompt: "第一版",
		Tools:        []string{"read_file"},
	}
	err = first.Create(initial)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(t, root, 0o700)
	assertPrivate(t, first.directory("小红"), 0o700)
	assertPrivate(t, first.revisionPath("小红", 1), 0o600)

	updated := initial
	updated.Revision = 2
	updated.SystemPrompt = "第二版"
	updated.Tools = []string{"write_file"}
	err = first.Update(updated)
	if err != nil {
		t.Fatal(err)
	}
	err = first.Archive("小红")
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(t, first.revisionPath("小红", 2), 0o600)
	assertPrivate(t, first.revisionPath("小红", 3), 0o600)

	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := second.Get("小红")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 3 || !current.Archived || current.SystemPrompt != "第二版" {
		t.Fatalf("重启后当前版本不对：%+v", current)
	}
	old, err := second.GetRevision("小红", 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Archived || old.SystemPrompt != "第一版" || old.Tools[0] != "read_file" {
		t.Fatalf("第 1 版不对：%+v", old)
	}
	beforeArchive, err := second.GetRevision("小红", 2)
	if err != nil {
		t.Fatal(err)
	}
	if beforeArchive.Archived || beforeArchive.Tools[0] != "write_file" {
		t.Fatalf("第 2 版不对：%+v", beforeArchive)
	}
	listed, err := second.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Revision != 3 {
		t.Fatalf("List 没有只给当前版本：%+v", listed)
	}
}

func TestStoreRejectsBrokenVersionSequence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invalid := presets.Revision{ID: "小红", Revision: 2}
	err = store.Create(invalid)
	if err == nil {
		t.Fatal("第 2 版不该能直接创建")
	}

	initial := invalid
	initial.Revision = 1
	err = store.Create(initial)
	if err != nil {
		t.Fatal(err)
	}
	invalid.Revision = 3
	err = store.Update(invalid)
	if err == nil {
		t.Fatal("跳版本不该能更新")
	}
	invalid.Revision = 2
	invalid.Archived = true
	err = store.Update(invalid)
	if err == nil {
		t.Fatal("Update 不该能改归档状态")
	}
	_, err = store.GetRevision("小红", 0)
	if err == nil {
		t.Fatal("版本 0 不该能读取")
	}
}

func TestStoreArchiveIsIdempotentAndKeepsOldRevision(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = store.Create(presets.Revision{ID: "小红", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Archive("小红")
	if err != nil {
		t.Fatal(err)
	}
	err = store.Archive("小红")
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Get("小红")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 2 || !current.Archived {
		t.Fatalf("重复归档不该追加版本：%+v", current)
	}
	old, err := store.GetRevision("小红", 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Archived {
		t.Fatalf("归档改写了第 1 版：%+v", old)
	}
}

func TestStoreUsesEncodedDirectoriesForPresetID(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := presets.Revision{ID: "团队/客服", Revision: 1}
	err = store.Create(revision)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(store.directory(revision.ID))
	if err != nil {
		t.Fatalf("模式 id 没有安全地编码为目录名：%v", err)
	}
	loaded, err := store.GetRevision(revision.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != revision.ID {
		t.Fatalf("编码目录后的模式 id 不对：%+v", loaded)
	}
}

func TestPluginRegistersPresetStore(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(Plugin{Root: t.TempDir()}, presets.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := presets.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = service.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Get("模式")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 {
		t.Fatalf("插件登记的模式库不对：%+v", loaded)
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
