package persist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 活对象。jsonl 账本和 SessionSettings 的实现。
type jsonl struct {
	dir string
	mu  sync.Mutex
}

func checkID(id string) error {
	if id == "" || id == "." || id == ".." {
		return fmt.Errorf("persist: bad id %q", id)
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("persist: bad id %q", id)
	}
	return nil
}

func openJSONL(dir string) (*jsonl, error) {
	if dir == "" {
		return nil, fmt.Errorf("persist: empty dir")
	}
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, err
	}
	return &jsonl{dir: dir}, nil
}

func (s *jsonl) treeFile(id string) string {
	return filepath.Join(s.sessionDir(id), "messages.jsonl")
}

func (s *jsonl) metaFile(id string) string {
	return filepath.Join(s.sessionDir(id), "meta.json")
}

func (s *jsonl) sessionDir(id string) string {
	return filepath.Join(s.dir, "sessions", id)
}

func (s *jsonl) ensureSessionDir(id string) error {
	return os.MkdirAll(s.sessionDir(id), 0o755)
}

func (s *jsonl) Load(id string) (*Tree, error) {
	err := checkID(id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.treeFile(id))
	if err != nil {
		return nil, err
	}

	tree := &Tree{ID: id}
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var n Node
		err := json.Unmarshal(line, &n)
		if err != nil {
			return nil, fmt.Errorf("persist: load %q: %w", id, err)
		}
		tree.Nodes = append(tree.Nodes, n)
	}
	return tree, nil
}

func (s *jsonl) Save(id string, tree *Tree) error {
	err := checkID(id)
	if err != nil {
		return err
	}
	if tree == nil {
		return fmt.Errorf("persist: save %q: nil tree", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.ensureSessionDir(id)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, n := range tree.Nodes {
		line, err := json.Marshal(n)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}

	path := s.treeFile(id)
	tmp := path + ".tmp"
	err = os.WriteFile(tmp, buf.Bytes(), 0o644)
	if err != nil {
		return err
	}
	err = os.Rename(tmp, path)
	if err != nil {
		return err
	}
	return nil
}

func (s *jsonl) LoadMeta(id string) (Meta, error) {
	err := checkID(id)
	if err != nil {
		return Meta{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	body, err := os.ReadFile(s.metaFile(id))
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	err = json.Unmarshal(body, &meta)
	if err != nil {
		return Meta{}, fmt.Errorf("persist: session meta %q: %w", id, err)
	}
	if meta.ID != id {
		return Meta{}, fmt.Errorf("persist: session meta %q has id %q", id, meta.ID)
	}
	return meta, nil
}

func (s *jsonl) SaveMeta(meta Meta) error {
	err := checkID(meta.ID)
	if err != nil {
		return err
	}
	if meta.Title == "" {
		return fmt.Errorf("persist: session meta %q has empty title", meta.ID)
	}
	if meta.CreatedAt.IsZero() {
		return fmt.Errorf("persist: session meta %q has empty created time", meta.ID)
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.ensureSessionDir(meta.ID)
	if err != nil {
		return err
	}

	path := s.metaFile(meta.ID)
	tmp := path + ".tmp"
	err = os.WriteFile(tmp, body, 0o644)
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *jsonl) DeleteMeta(id string) error {
	err := checkID(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return os.Remove(s.metaFile(id))
}

func (s *jsonl) List() ([]Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	matches, err := filepath.Glob(filepath.Join(s.dir, "sessions", "*", "meta.json"))
	if err != nil {
		return nil, err
	}

	out := make([]Meta, 0, len(matches))
	for _, path := range matches {
		name := filepath.Base(path)
		expectedID := filepath.Base(filepath.Dir(path))
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var meta Meta
		err = json.Unmarshal(body, &meta)
		if err != nil {
			return nil, fmt.Errorf("persist: session meta %q: %w", expectedID, err)
		}
		err = checkID(meta.ID)
		if err != nil {
			return nil, err
		}
		if meta.ID != expectedID {
			return nil, fmt.Errorf("persist: session meta %q has id %q", name, meta.ID)
		}
		out = append(out, meta)
	}
	return out, nil
}

func (s *jsonl) Add(id string, node Node) error {
	err := checkID(id)
	if err != nil {
		return err
	}
	if node.ID == "" {
		return fmt.Errorf("persist: empty node id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.ensureSessionDir(id)
	if err != nil {
		return err
	}

	line, err := json.Marshal(node)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.treeFile(id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(append(line, '\n'))
	if err != nil {
		return err
	}
	err = f.Sync()
	if err != nil {
		return err
	}
	return nil
}
