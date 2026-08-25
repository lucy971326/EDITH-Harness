package projectjson

import (
	"os"
	"testing"

	"harness/projects"
)

func TestStorePersistsProjectAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	projectRoot := t.TempDir()
	first, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	project := projects.Project{
		ID:           "demo",
		Name:         "演示项目",
		Root:         projectRoot,
		LastPresetID: "chat",
	}
	err = first.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(t, directory, 0o700)
	assertPrivate(t, first.path("demo"), 0o600)

	second, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := second.Get("demo")
	if err != nil {
		t.Fatal(err)
	}
	if loaded != project {
		t.Fatalf("重启后项目不对：%+v", loaded)
	}

	loaded.Name = "新名称"
	loaded.LastPresetID = "coding"
	loaded.Archived = true
	err = second.Update(loaded)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := second.Get("demo")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "新名称" || updated.LastPresetID != "coding" || !updated.Archived {
		t.Fatalf("更新后项目不对：%+v", updated)
	}
}

func TestStoreRejectsRootChangeAndDuplicateID(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := projects.Project{ID: "demo", Name: "演示项目", Root: t.TempDir()}
	err = store.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Create(project)
	if err == nil {
		t.Fatal("重复 id 可以覆盖项目")
	}
	project.Root = t.TempDir()
	err = store.Update(project)
	if err == nil {
		t.Fatal("项目根目录可以修改")
	}
}

func TestStoreListsProjectsByID(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = store.Create(projects.Project{ID: "b", Name: "乙", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Create(projects.Project{ID: "a", Name: "甲", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "a" || listed[1].ID != "b" {
		t.Fatalf("项目排序不对：%+v", listed)
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
