package presets

import (
	"fmt"
	"sync"
	"testing"
)

type memoryStore struct {
	mu       sync.Mutex
	versions map[string][]Revision
}

func newMemoryStore() *memoryStore {
	return &memoryStore{versions: make(map[string][]Revision)}
}

func (s *memoryStore) Create(revision Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.versions[revision.ID]; exists {
		return fmt.Errorf("模式 %s 已存在", revision.ID)
	}
	s.versions[revision.ID] = []Revision{clone(revision)}
	return nil
}

func (s *memoryStore) Update(revision Revision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions, exists := s.versions[revision.ID]
	if !exists {
		return fmt.Errorf("模式 %s 不存在", revision.ID)
	}
	current := versions[len(versions)-1]
	if revision.Revision != current.Revision+1 {
		return fmt.Errorf("模式 %s 版本不连续", revision.ID)
	}
	s.versions[revision.ID] = append(versions, clone(revision))
	return nil
}

func (s *memoryStore) Get(id string) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions, exists := s.versions[id]
	if !exists {
		return Revision{}, fmt.Errorf("模式 %s 不存在", id)
	}
	return clone(versions[len(versions)-1]), nil
}

func (s *memoryStore) GetRevision(id string, number int) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions, exists := s.versions[id]
	if !exists || number < 1 || number > len(versions) {
		return Revision{}, fmt.Errorf("模式 %s 的版本 %d 不存在", id, number)
	}
	return clone(versions[number-1]), nil
}

func (s *memoryStore) List() ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := make([]Revision, 0, len(s.versions))
	for _, versions := range s.versions {
		listed = append(listed, clone(versions[len(versions)-1]))
	}
	return listed, nil
}

func (s *memoryStore) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions, exists := s.versions[id]
	if !exists {
		return fmt.Errorf("模式 %s 不存在", id)
	}
	current := versions[len(versions)-1]
	if current.Archived {
		return nil
	}
	current.Archived = true
	current.Revision++
	s.versions[id] = append(versions, current)
	return nil
}

func TestServiceKeepsHistoricalRevisions(t *testing.T) {
	service := New(newMemoryStore())
	err := service.Create(Preset{
		ID:           "客服",
		Revision:     99,
		Provider:     "deepseek",
		Model:        "chat",
		Thinking:     "off",
		SystemPrompt: "第一版",
		Tools:        []string{"read_file"},
		Archived:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.GetRevision("客服", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || first.Archived {
		t.Fatalf("创建没有重置初始版本：%+v", first)
	}

	err = service.Update(Preset{
		ID:           "客服",
		Revision:     42,
		Provider:     "deepseek",
		Model:        "reasoner",
		Thinking:     "high",
		SystemPrompt: "第二版",
		Tools:        []string{"write_file"},
		Archived:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Get("客服")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 2 || current.Archived || current.Model != "reasoner" {
		t.Fatalf("更新没有生成正确的当前版本：%+v", current)
	}
	first, err = service.GetRevision("客服", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Model != "chat" || first.SystemPrompt != "第一版" || first.Tools[0] != "read_file" {
		t.Fatalf("旧版本被改写：%+v", first)
	}

	err = service.Archive("客服")
	if err != nil {
		t.Fatal(err)
	}
	archived, err := service.Get("客服")
	if err != nil {
		t.Fatal(err)
	}
	if archived.Revision != 3 || !archived.Archived {
		t.Fatalf("归档没有追加版本：%+v", archived)
	}
	second, err := service.GetRevision("客服", 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Archived || second.Model != "reasoner" {
		t.Fatalf("归档改写了旧版本：%+v", second)
	}
}

func TestServiceSerializesVersionAllocation(t *testing.T) {
	service := New(newMemoryStore())
	err := service.Create(Preset{ID: "串行", Provider: "test", Model: "m", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}

	var group sync.WaitGroup
	errs := make(chan error, 2)
	for _, model := range []string{"m2", "m3"} {
		group.Add(1)
		go func(model string) {
			defer group.Done()
			errs <- service.Update(Preset{ID: "串行", Provider: "test", Model: model, Thinking: "off"})
		}(model)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := service.Get("串行")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 3 {
		t.Fatalf("并发更新后的版本是 %d，想要 3", current.Revision)
	}
}

func TestServiceReturnsIndependentCopies(t *testing.T) {
	service := New(newMemoryStore())
	err := service.Create(Preset{ID: "副本", Provider: "test", Model: "m", Thinking: "off", Tools: []string{"read_file"}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Get("副本")
	if err != nil {
		t.Fatal(err)
	}
	loaded.Tools[0] = "write_file"
	again, err := service.Get("副本")
	if err != nil {
		t.Fatal(err)
	}
	if again.Tools[0] != "read_file" {
		t.Fatalf("返回的工具切片泄漏到存储：%+v", again)
	}
}
