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
	return filepath.Join(s.dir, id+".jsonl")
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

func (s *jsonl) List() ([]Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	matches, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	out := make([]Meta, 0, len(matches))
	for _, path := range matches {
		name := filepath.Base(path)
		id := strings.TrimSuffix(name, ".jsonl")
		out = append(out, Meta{ID: id})
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
