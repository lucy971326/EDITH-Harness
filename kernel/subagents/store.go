package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const currentTaskVersion = 1

// 活对象。管理 ~/.harness/subagents/ 下任务的版本化 JSON 独立存储。
type taskStore struct {
	dir string
	mu  sync.Mutex
}

func newTaskStore(dir string) (*taskStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("subagents store: empty dir")
	}
	tasksDir := filepath.Join(dir, "tasks")
	err := os.MkdirAll(tasksDir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("subagents store: create tasks dir: %w", err)
	}
	return &taskStore{dir: tasksDir}, nil
}

func (s *taskStore) taskPath(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("subagents store: bad id %q", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func validateTask(task Task, expectedID string) error {
	if task.Version != currentTaskVersion {
		return fmt.Errorf("%w: version %d must be %d", ErrUnsupportedVersion, task.Version, currentTaskVersion)
	}
	if task.ID == "" {
		return fmt.Errorf("%w: empty task ID", ErrInvalidTaskData)
	}
	if expectedID != "" && task.ID != expectedID {
		return fmt.Errorf("%w: task ID %q does not match filename ID %q", ErrInvalidTaskData, task.ID, expectedID)
	}
	if task.ParentSessionID == "" {
		return fmt.Errorf("%w: empty parentSessionID", ErrInvalidTaskData)
	}
	if task.ChildSessionID == "" {
		return fmt.Errorf("%w: empty childSessionID", ErrInvalidTaskData)
	}
	switch task.Status {
	case StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted:
	default:
		return fmt.Errorf("%w: invalid task status %q", ErrInvalidTaskData, task.Status)
	}
	if task.Turn < 1 {
		return fmt.Errorf("%w: turn %d must be >= 1", ErrInvalidTaskData, task.Turn)
	}
	if len(task.Turns) != task.Turn {
		return fmt.Errorf("%w: missing turn history", ErrInvalidTaskData)
	}
	for i, rec := range task.Turns {
		if rec.Turn != i+1 {
			return fmt.Errorf("%w: nonsequential turn history", ErrInvalidTaskData)
		}
		switch rec.Status {
		case StatusPending, StatusRunning:
			if i != len(task.Turns)-1 {
				return fmt.Errorf("%w: unfinished historical turn", ErrInvalidTaskData)
			}
		case StatusCompleted, StatusFailed, StatusCancelled, StatusInterrupted:
		default:
			return fmt.Errorf("%w: invalid turn status", ErrInvalidTaskData)
		}
		if (rec.Status == StatusRunning || rec.Status == StatusCompleted) && rec.RunID == "" {
			return fmt.Errorf("%w: missing run ID", ErrInvalidTaskData)
		}
	}
	last := task.Turns[len(task.Turns)-1]
	if last.Status != task.Status || last.RunID != task.CurrentRunID || last.ResultEntryID != task.ResultEntryID {
		return fmt.Errorf("%w: current task disagrees with turn history", ErrInvalidTaskData)
	}
	return nil
}

// saveTask 原子保存一条任务记录。
func (s *taskStore) saveTask(task Task) error {
	err := validateTask(task, "")
	if err != nil {
		return err
	}

	path, err := s.taskPath(task.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("subagents store: marshal task %q: %w", task.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("subagents store: create temp file: %w", err)
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: write task %q: %w", task.ID, writeErr)
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: sync task %q: %w", task.ID, syncErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: close task %q: %w", task.ID, closeErr)
	}

	// 目录项落盘保证：rename 前同步目录
	dirF, dirErr := os.Open(s.dir)
	if dirErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: open dir for sync: %w", dirErr)
	}
	syncDirErr := dirF.Sync()
	closeDirErr := dirF.Close()
	if syncDirErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: sync dir: %w", syncDirErr)
	}
	if closeDirErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: close dir: %w", closeDirErr)
	}

	err = os.Rename(tmp, path)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subagents store: rename task %q: %w", task.ID, err)
	}

	// 目录项落盘保证：rename 后再次同步目录
	dirF2, dirErr2 := os.Open(s.dir)
	if dirErr2 != nil {
		return fmt.Errorf("subagents store: open dir post-rename: %w", dirErr2)
	}
	syncDirErr2 := dirF2.Sync()
	closeDirErr2 := dirF2.Close()
	if syncDirErr2 != nil {
		return fmt.Errorf("subagents store: sync dir post-rename: %w", syncDirErr2)
	}
	if closeDirErr2 != nil {
		return fmt.Errorf("subagents store: close dir post-rename: %w", closeDirErr2)
	}

	return nil
}

// loadTask 读取指定任务；遇到未知版本或校验不通过严格报错。
func (s *taskStore) loadTask(id string) (Task, error) {
	path, err := s.taskPath(id)
	if err != nil {
		return Task{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	var task Task
	err = json.Unmarshal(data, &task)
	if err != nil {
		return Task{}, fmt.Errorf("subagents store: unmarshal task %q: %w", id, err)
	}
	err = validateTask(task, id)
	if err != nil {
		return Task{}, fmt.Errorf("subagents store: validate task %q: %w", id, err)
	}
	return task, nil
}

// listTasks 列出全部已持久化任务；遇到读错、校验不通过或未知版本严格报错，绝不静默丢记录。
func (s *taskStore) listTasks() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("subagents store: read dir: %w", err)
	}

	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		expectedID := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("subagents store: read task file %q: %w", entry.Name(), err)
		}
		var task Task
		err = json.Unmarshal(data, &task)
		if err != nil {
			return nil, fmt.Errorf("subagents store: unmarshal task file %q: %w", entry.Name(), err)
		}
		err = validateTask(task, expectedID)
		if err != nil {
			return nil, fmt.Errorf("subagents store: validate task file %q: %w", entry.Name(), err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
