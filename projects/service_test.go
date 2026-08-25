package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"harness/session"
)

func TestServiceManagesProjectLifecycle(t *testing.T) {
	root := t.TempDir()
	store := newMemoryStore()
	books := &memoryHeaders{}
	service := New(store, books)

	created, err := service.Create(root)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("项目 id 没有自动生成")
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if created.Root != wantRoot {
		t.Fatalf("项目根目录 = %q，想要 %q", created.Root, wantRoot)
	}
	if created.Name != filepath.Base(wantRoot) {
		t.Fatalf("项目名称 = %q，想要目录名 %q", created.Name, filepath.Base(wantRoot))
	}
	err = service.RememberPreset(created.ID, "code-review")
	if err != nil {
		t.Fatal(err)
	}
	err = service.Archive(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := service.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.LastPresetID != "code-review" || !archived.Archived {
		t.Fatalf("项目状态不对：%+v", archived)
	}
	err = service.Restore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Archived {
		t.Fatalf("项目应已恢复：%+v", restored)
	}
}

func TestServiceNormalizesRootAndRejectsDuplicate(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "project-link")
	err := os.Symlink(root, link)
	if err != nil {
		t.Fatal(err)
	}
	service := New(newMemoryStore(), &memoryHeaders{})
	first, err := service.Create(link)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Root != wantRoot {
		t.Fatalf("符号链接没有规范化：%q", first.Root)
	}
	_, err = service.Create(root)
	if err == nil {
		t.Fatal("同一个规范化目录可以重复登记")
	}
}

func TestServiceRejectsFileAsRoot(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-directory")
	if err != nil {
		t.Fatal(err)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	service := New(newMemoryStore(), &memoryHeaders{})
	_, err = service.Create(file.Name())
	if err == nil {
		t.Fatal("普通文件可以作为项目根目录")
	}
}

func TestServiceListsProjectsByID(t *testing.T) {
	store := newMemoryStore()
	service := New(store, &memoryHeaders{})
	err := store.Create(Project{ID: "2", Name: "乙", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = store.Create(Project{ID: "1", Name: "甲", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "1" || listed[1].ID != "2" {
		t.Fatalf("项目排序不对：%+v", listed)
	}
}

func TestServiceListsOnlyProjectSessions(t *testing.T) {
	store := newMemoryStore()
	err := store.Create(Project{ID: "project-a", Name: "A", Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	books := &memoryHeaders{headers: []session.Header{
		{ID: "a-1", ProjectID: "project-a", CreatedAt: time.Now()},
		{ID: "b-1", ProjectID: "project-b", CreatedAt: time.Now()},
		{ID: "a-2", ProjectID: "project-a", CreatedAt: time.Now()},
	}}
	service := New(store, books)
	listed, err := service.ListSessions("project-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != "a-1" || listed[1].ID != "a-2" {
		t.Fatalf("会话筛选不对：%+v", listed)
	}
}

type memoryStore struct {
	projects map[string]Project
}

func newMemoryStore() *memoryStore {
	return &memoryStore{projects: make(map[string]Project)}
}

func (s *memoryStore) Create(project Project) error {
	if _, exists := s.projects[project.ID]; exists {
		return fmt.Errorf("项目已存在")
	}
	s.projects[project.ID] = project
	return nil
}

func (s *memoryStore) Get(id string) (Project, error) {
	project, exists := s.projects[id]
	if !exists {
		return Project{}, fmt.Errorf("项目不存在")
	}
	return project, nil
}

func (s *memoryStore) List() ([]Project, error) {
	listed := make([]Project, 0, len(s.projects))
	for _, project := range s.projects {
		listed = append(listed, project)
	}
	return listed, nil
}

func (s *memoryStore) Update(project Project) error {
	if _, exists := s.projects[project.ID]; !exists {
		return fmt.Errorf("项目不存在")
	}
	s.projects[project.ID] = project
	return nil
}

type memoryHeaders struct {
	headers []session.Header
}

func (s *memoryHeaders) ListHeaders() ([]session.Header, error) {
	return append([]session.Header(nil), s.headers...), nil
}
