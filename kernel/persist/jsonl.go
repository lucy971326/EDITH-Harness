package persist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// 活对象。jsonl 账本和 SessionSettings 的实现。
type jsonl struct {
	files *Files
	mu    sync.Mutex
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
	files, err := NewFiles(dir)
	if err != nil {
		return nil, err
	}
	return newJSONL(files), nil
}

func newJSONL(files *Files) *jsonl {
	return &jsonl{files: files}
}

func (s *jsonl) sessionFiles(id string) (*Files, error) {
	return s.files.Scope("sessions", id)
}

func (s *jsonl) Load(id string) (*Tree, error) {
	err := checkID(id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sessionFiles(id)
	if err != nil {
		return nil, err
	}
	b, err := files.Read("messages.jsonl")
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
	files, err := s.sessionFiles(id)
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

	return files.Write("messages.jsonl", buf.Bytes())
}

func (s *jsonl) LoadMeta(id string) (Meta, error) {
	err := checkID(id)
	if err != nil {
		return Meta{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sessionFiles(id)
	if err != nil {
		return Meta{}, err
	}
	body, err := files.Read("meta.json")
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
	files, err := s.sessionFiles(meta.ID)
	if err != nil {
		return err
	}
	return files.Write("meta.json", body)
}

func (s *jsonl) DeleteMeta(id string) error {
	err := checkID(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sessionFiles(id)
	if err != nil {
		return err
	}
	return files.Remove("meta.json")
}

func (s *jsonl) List() ([]Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.files.Scope("sessions")
	if err != nil {
		return nil, err
	}
	entries, err := sessions.List()
	if err != nil {
		return nil, err
	}
	out := make([]Meta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		expectedID := entry.Name
		files, err := sessions.Scope(expectedID)
		if err != nil {
			return nil, err
		}
		body, err := files.Read("meta.json")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
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
			return nil, fmt.Errorf("persist: session meta %q has id %q", expectedID, meta.ID)
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
	files, err := s.sessionFiles(id)
	if err != nil {
		return err
	}

	line, err := json.Marshal(node)
	if err != nil {
		return err
	}

	return files.Append("messages.jsonl", append(line, '\n'))
}

func (s *jsonl) LoadRunRecords(id string) ([]byte, error) {
	err := checkID(id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sessionFiles(id)
	if err != nil {
		return nil, err
	}
	body, err := files.Read("runs.json")
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *jsonl) SaveRunRecords(id string, body []byte) error {
	err := checkID(id)
	if err != nil {
		return err
	}
	if body == nil {
		body = []byte("[]")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.sessionFiles(id)
	if err != nil {
		return err
	}
	return files.Write("runs.json", body)
}
