package machinelocal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"harness/kernel/machine"
)

type pathLock struct {
	gate chan struct{}
	refs int
}

type pathLockSet struct {
	mu    sync.Mutex
	items map[string]*pathLock
}

func (s *pathLockSet) lock(path string) func() {
	unlock, _ := s.lockKey(context.Background(), canonicalPath(path))
	return unlock
}

// 批量调用者先固定、排序规范化 key；等待期间不再解析可能变化的路径。
func (s *pathLockSet) lockKey(ctx context.Context, key string) (func(), error) {
	s.mu.Lock()
	if s.items == nil {
		s.items = make(map[string]*pathLock)
	}
	item := s.items[key]
	if item == nil {
		item = &pathLock{gate: make(chan struct{}, 1)}
		s.items[key] = item
	}
	item.refs++
	s.mu.Unlock()

	release := func() {
		s.mu.Lock()
		item.refs--
		if item.refs == 0 {
			delete(s.items, key)
		}
		s.mu.Unlock()
	}
	select {
	case item.gate <- struct{}{}:
		return func() { <-item.gate; release() }, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	abs = filepath.Clean(abs)

	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = filepath.Clean(resolved)
	} else {
		// 文件还不存在时，先解析最近的现存父目录，再接回剩余路径。
		parent := abs
		var remaining []string
		for {
			_, statErr := os.Lstat(parent)
			if statErr == nil {
				resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
				if resolveErr == nil {
					for i := len(remaining) - 1; i >= 0; i-- {
						resolvedParent = filepath.Join(resolvedParent, remaining[i])
					}
					abs = filepath.Clean(resolvedParent)
				}
				break
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			remaining = append(remaining, filepath.Base(parent))
			parent = next
		}
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(abs)
	}
	return abs
}

func fileHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Local) ReadFile(path string) ([]byte, error) {
	content, err := m.ReadFileVersion(path, 0)
	if err != nil {
		return nil, err
	}
	return content.Data, nil
}

func (m *Local) ReadFileVersion(path string, maxBytes int64) (machine.FileContent, error) {
	unlock := m.fileLocks.lock(path)
	defer unlock()
	return readFileVersion(path, maxBytes)
}

func readFileVersion(path string, maxBytes int64) (machine.FileContent, error) {
	file, err := os.Open(path)
	if err != nil {
		return machine.FileContent{}, fmt.Errorf("machine-local: read %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return machine.FileContent{}, fmt.Errorf("machine-local: inspect %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return machine.FileContent{}, fmt.Errorf("machine-local: read %q: %w", path, machine.ErrNotRegularFile)
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return machine.FileContent{}, fmt.Errorf("machine-local: read %q: %w", path, machine.ErrFileTooLarge)
	}

	reader := io.Reader(file)
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return machine.FileContent{}, fmt.Errorf("machine-local: read %q: %w", path, err)
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return machine.FileContent{}, fmt.Errorf("machine-local: read %q: %w", path, machine.ErrFileTooLarge)
	}
	return machine.FileContent{Data: data, Hash: fileHash(data)}, nil
}

func (m *Local) ReadDir(path string) ([]machine.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("machine-local: read directory %q: %w", path, err)
	}
	out := make([]machine.DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, machine.DirEntry{
			Name:   entry.Name(),
			IsDir:  entry.IsDir(),
			IsFile: entry.Type().IsRegular(),
		})
	}
	return out, nil
}

func (m *Local) Metadata(path string) (machine.FileMetadata, error) {
	unlock := m.fileLocks.lock(path)
	defer unlock()

	linkInfo, err := os.Lstat(path)
	if err != nil {
		return machine.FileMetadata{}, fmt.Errorf("machine-local: inspect %q: %w", path, err)
	}
	info := linkInfo
	isSymlink := linkInfo.Mode()&os.ModeSymlink != 0
	if isSymlink {
		info, err = os.Stat(path)
		if err != nil {
			return machine.FileMetadata{}, fmt.Errorf("machine-local: inspect symlink %q: %w", path, err)
		}
	}
	return machine.FileMetadata{
		IsDir:        info.IsDir(),
		IsFile:       info.Mode().IsRegular(),
		IsSymlink:    isSymlink,
		ModifiedAtMS: info.ModTime().UnixMilli(),
	}, nil
}

func (m *Local) WriteFile(path string, data []byte) error {
	unlock := m.fileLocks.lock(path)
	defer unlock()

	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("machine-local: create parent for %q: %w", path, err)
	}
	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("machine-local: write %q: %w", path, err)
	}
	return nil
}

func (m *Local) WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error) {
	unlock := m.fileLocks.lock(path)
	defer unlock()

	if expectedHash == machine.AbsentFileHash {
		dir := filepath.Dir(path)
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			return "", fmt.Errorf("machine-local: create parent for %q: %w", path, err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("machine-local: write %q: %w", path, machine.ErrFileConflict)
		}
		if err != nil {
			return "", fmt.Errorf("machine-local: create %q: %w", path, err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("machine-local: write new file %q: %w", path, errors.Join(writeErr, closeErr))
		}
		return fileHash(data), nil
	}

	current, err := readFileVersion(path, 0)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(current.Hash, expectedHash) {
		return "", fmt.Errorf("machine-local: write %q: %w", path, machine.ErrFileConflict)
	}
	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return "", fmt.Errorf("machine-local: write %q: %w", path, err)
	}
	return fileHash(data), nil
}

func (m *Local) RemoveFileIfUnchanged(path string, expectedHash string) error {
	unlock := m.fileLocks.lock(path)
	defer unlock()

	current, err := readFileVersion(path, 0)
	if err != nil {
		return err
	}
	if !strings.EqualFold(current.Hash, expectedHash) {
		return fmt.Errorf("machine-local: remove %q: %w", path, machine.ErrFileConflict)
	}
	err = os.Remove(path)
	if err != nil {
		return fmt.Errorf("machine-local: remove %q: %w", path, err)
	}
	return nil
}
