package persist

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// 数据。持久化目录中的一个直接子项。
type File struct {
	Name  string
	IsDir bool
}

// 活对象。固定使用本机文件系统，并把所有路径限制在自己的根目录内。
// Scope 返回共享同一把写入锁的子目录视图，供各模块隔离自己的文件。
type Files struct {
	root  string
	state *fileState
}

type fileState struct {
	mu sync.Mutex
}

// NewFiles 打开固定的数据根目录。
func NewFiles(root string) (*Files, error) {
	if root == "" {
		return nil, fmt.Errorf("persist: empty root")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("persist: resolve root: %w", err)
	}
	err = ensureDirectory(absolute)
	if err != nil {
		return nil, fmt.Errorf("persist: create root: %w", err)
	}
	return &Files{root: absolute, state: &fileState{}}, nil
}

// Scope 返回限定在直接子目录内的存储视图。
func (f *Files) Scope(names ...string) (*Files, error) {
	if f == nil || f.state == nil {
		return nil, fmt.Errorf("persist: nil files")
	}
	root := f.root
	for _, name := range names {
		err := checkName(name)
		if err != nil {
			return nil, err
		}
		root = filepath.Join(root, name)
	}
	return &Files{root: root, state: f.state}, nil
}

// Path 返回根目录或其中一个直接子项的绝对路径，只用于必须向外部暴露文件位置的场景。
func (f *Files) Path(name ...string) (string, error) {
	if f == nil || f.state == nil {
		return "", fmt.Errorf("persist: nil files")
	}
	path := f.root
	for _, part := range name {
		err := checkName(part)
		if err != nil {
			return "", err
		}
		path = filepath.Join(path, part)
	}
	return path, nil
}

// Ensure 创建当前作用域目录。
func (f *Files) Ensure() error {
	if f == nil || f.state == nil {
		return fmt.Errorf("persist: nil files")
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return ensureDirectory(f.root)
}

// Read 读取一个直接子文件。
func (f *Files) Read(name string) ([]byte, error) {
	path, err := f.childPath(name)
	if err != nil {
		return nil, err
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return os.ReadFile(path)
}

// List 列出当前作用域的直接子项；目录尚不存在时返回空列表。
func (f *Files) List() ([]File, error) {
	if f == nil || f.state == nil {
		return nil, fmt.Errorf("persist: nil files")
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	entries, err := os.ReadDir(f.root)
	if errors.Is(err, os.ErrNotExist) {
		return []File{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(entries))
	for _, entry := range entries {
		out = append(out, File{Name: entry.Name(), IsDir: entry.IsDir()})
	}
	return out, nil
}

// Write 通过同目录临时文件原子替换一个直接子文件。
func (f *Files) Write(name string, data []byte) error {
	path, err := f.childPath(name)
	if err != nil {
		return err
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()

	err = os.MkdirAll(f.root, 0o700)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(f.root, ".harness-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	err = temporary.Chmod(0o600)
	if err == nil {
		_, err = io.Copy(temporary, bytes.NewReader(data))
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	err = os.Rename(temporaryPath, path)
	if err != nil {
		return err
	}
	return syncDirectory(f.root)
}

// Append 追加一个直接子文件并在返回前同步数据。
func (f *Files) Append(name string, data []byte) error {
	path, err := f.childPath(name)
	if err != nil {
		return err
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()

	err = os.MkdirAll(f.root, 0o700)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	chmodErr := file.Chmod(0o600)
	if chmodErr != nil {
		_ = file.Close()
		return chmodErr
	}
	_, writeErr := io.Copy(file, bytes.NewReader(data))
	syncErr := file.Sync()
	closeErr := file.Close()
	directoryErr := syncDirectory(f.root)
	return errors.Join(writeErr, syncErr, closeErr, directoryErr)
}

// Remove 删除一个直接子文件。
func (f *Files) Remove(name string) error {
	path, err := f.childPath(name)
	if err != nil {
		return err
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	err = os.Remove(path)
	if err != nil {
		return err
	}
	return syncDirectory(f.root)
}

func (f *Files) childPath(name string) (string, error) {
	if f == nil || f.state == nil {
		return "", fmt.Errorf("persist: nil files")
	}
	err := checkName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(f.root, name), nil
}

func checkName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("persist: bad file name %q", name)
	}
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

func ensureDirectory(path string) error {
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}
