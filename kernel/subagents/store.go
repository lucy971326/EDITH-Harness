package subagents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"harness/kernel/persist"
)

const currentTaskVersion = 1

// 活对象。管理 ~/.harness/subagents/tasks/ 下的父子 Session 关系。
type taskStore struct {
	files *persist.Files
	dir   string
	mu    sync.Mutex
}

func newTaskStore(dir string) (*taskStore, error) {
	files, err := persist.NewFiles(dir)
	if err != nil {
		return nil, err
	}
	tasks, err := files.Scope("tasks")
	if err != nil {
		return nil, err
	}
	return newTaskStoreFiles(tasks)
}

func newTaskStoreFiles(files *persist.Files) (*taskStore, error) {
	if files == nil {
		return nil, fmt.Errorf("subagents store: nil files")
	}
	err := files.Ensure()
	if err != nil {
		return nil, fmt.Errorf("subagents store: create tasks dir: %w", err)
	}
	dir, err := files.Path()
	if err != nil {
		return nil, err
	}
	return &taskStore{files: files, dir: dir}, nil
}

func (s *taskStore) taskName(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("subagents store: bad id %q", id)
	}
	return id + ".json", nil
}

func validateTask(record TaskRecord, expectedID string) error {
	if record.Version != currentTaskVersion {
		return fmt.Errorf("%w: version %d must be %d", ErrUnsupportedVersion, record.Version, currentTaskVersion)
	}
	if record.ID == "" {
		return fmt.Errorf("%w: empty task ID", ErrInvalidTaskData)
	}
	if expectedID != "" && record.ID != expectedID {
		return fmt.Errorf("%w: task ID %q does not match filename ID %q", ErrInvalidTaskData, record.ID, expectedID)
	}
	if record.ParentSessionID == "" {
		return fmt.Errorf("%w: empty parentSessionID", ErrInvalidTaskData)
	}
	if record.ChildSessionID == "" {
		return fmt.Errorf("%w: empty childSessionID", ErrInvalidTaskData)
	}
	if strings.TrimSpace(record.Description) == "" {
		return fmt.Errorf("%w: empty description", ErrInvalidTaskData)
	}
	return nil
}

func decodeTask(data []byte, expectedID string) (TaskRecord, error) {
	var record TaskRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&record)
	if err != nil {
		return TaskRecord{}, err
	}
	var trailing any
	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return TaskRecord{}, fmt.Errorf("trailing task data: %w", err)
	}
	err = validateTask(record, expectedID)
	if err != nil {
		return TaskRecord{}, err
	}
	return record, nil
}

// saveTask 原子保存一条关系记录。
func (s *taskStore) saveTask(record TaskRecord) error {
	err := validateTask(record, "")
	if err != nil {
		return err
	}
	name, err := s.taskName(record.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("subagents store: marshal task %q: %w", record.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.files.Write(name, data)
	if err != nil {
		return fmt.Errorf("subagents store: save task %q: %w", record.ID, err)
	}
	return nil
}

func (s *taskStore) loadTask(id string) (TaskRecord, error) {
	name, err := s.taskName(id)
	if err != nil {
		return TaskRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.files.Read(name)
	if err != nil {
		return TaskRecord{}, err
	}
	record, err := decodeTask(data, id)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("subagents store: decode task %q: %w", id, err)
	}
	return record, nil
}

// listTasks 列出全部关系；旧格式和损坏记录严格报错。
func (s *taskStore) listTasks() ([]TaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.files.List()
	if err != nil {
		return nil, fmt.Errorf("subagents store: read dir: %w", err)
	}

	records := make([]TaskRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		expectedID := strings.TrimSuffix(entry.Name, ".json")
		data, err := s.files.Read(entry.Name)
		if err != nil {
			return nil, fmt.Errorf("subagents store: read task file %q: %w", entry.Name, err)
		}
		record, err := decodeTask(data, expectedID)
		if err != nil {
			return nil, fmt.Errorf("subagents store: decode task file %q: %w", entry.Name, err)
		}
		records = append(records, record)
	}
	return records, nil
}
