package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"harness/kernel/persist"
	"os"
	"strings"
)

// 活对象。会话账本与元数据的文件格式。
type jsonl struct {
	files *persist.Files
}

func checkID(id string) error {
	if id == "" || id == "." || id == ".." {
		return fmt.Errorf("session: bad id %q", id)
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("session: bad id %q", id)
	}
	return nil
}

// NewPersistence 创建会话账本的文件存储；文件操作由 persist 串行写入。
func NewPersistence(files *persist.Files) Persistence {
	return &jsonl{files: files}
}

func (s *jsonl) sessionFiles(id string) (*persist.Files, error) {
	return s.files.Scope("sessions", id)
}

func (s *jsonl) Load(id string) (*Tree, error) {
	err := checkID(id)
	if err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("session: load %q: %w", id, err)
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
		return fmt.Errorf("session: save %q: nil tree", id)
	}

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

func (s *jsonl) LoadMeta(id string) (SessionMeta, error) {
	err := checkID(id)
	if err != nil {
		return SessionMeta{}, err
	}

	files, err := s.sessionFiles(id)
	if err != nil {
		return SessionMeta{}, err
	}
	body, err := files.Read("meta.json")
	if err != nil {
		return SessionMeta{}, err
	}
	var meta SessionMeta
	err = json.Unmarshal(body, &meta)
	if err != nil {
		return SessionMeta{}, fmt.Errorf("session: session meta %q: %w", id, err)
	}
	if meta.ID != id {
		return SessionMeta{}, fmt.Errorf("session: session meta %q has id %q", id, meta.ID)
	}
	return meta, nil
}

func (s *jsonl) SaveMeta(meta SessionMeta) error {
	err := checkID(meta.ID)
	if err != nil {
		return err
	}
	if meta.Title == "" {
		return fmt.Errorf("session: session meta %q has empty title", meta.ID)
	}
	if meta.CreatedAt.IsZero() {
		return fmt.Errorf("session: session meta %q has empty created time", meta.ID)
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}

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

	files, err := s.sessionFiles(id)
	if err != nil {
		return err
	}
	return files.Remove("meta.json")
}

func (s *jsonl) List() ([]SessionMeta, error) {
	sessions, err := s.files.Scope("sessions")
	if err != nil {
		return nil, err
	}
	entries, err := sessions.List()
	if err != nil {
		return nil, err
	}
	out := make([]SessionMeta, 0, len(entries))
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
		var meta SessionMeta
		err = json.Unmarshal(body, &meta)
		if err != nil {
			return nil, fmt.Errorf("session: session meta %q: %w", expectedID, err)
		}
		err = checkID(meta.ID)
		if err != nil {
			return nil, err
		}
		if meta.ID != expectedID {
			return nil, fmt.Errorf("session: session meta %q has id %q", expectedID, meta.ID)
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
		return fmt.Errorf("session: empty node id")
	}

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
