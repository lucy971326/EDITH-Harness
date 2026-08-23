package localenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"harness/workspace/files"
)

// localStore 把 files.Store 直接落到本地磁盘；根目录只是工作范围，不是安全沙箱。
type localStore struct {
	root string // 所有相对路径的起点
}

// newFileStore 返回一个以 root 为工作目录的本地文件能力。
func newFileStore(root string) (files.Store, error) {
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析文件根目录失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开文件根目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("文件根路径 %s 不是目录", absolute)
	}
	return &localStore{root: absolute}, nil
}

// Read 读取根目录下的一个文件。
func (s *localStore) Read(ctx context.Context, name string) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	path, err := s.resolveFile(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读文件 %s 失败：%w", name, err)
	}
	return data, nil
}

// Write 创建或覆盖根目录下的文件，不存在的父目录会一并创建。
func (s *localStore) Write(ctx context.Context, name string, data []byte) error {
	err := ctx.Err()
	if err != nil {
		return err
	}
	path, err := s.resolveFile(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return fmt.Errorf("创建文件 %s 的父目录失败：%w", name, err)
	}
	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("写文件 %s 失败：%w", name, err)
	}
	return nil
}

// List 列出根目录下一层内容，结果按名字排序。
func (s *localStore) List(ctx context.Context, name string) ([]files.Entry, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	path, err := s.resolveFile(name)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("列目录 %s 失败：%w", name, err)
	}

	entries := make([]files.Entry, 0, len(items))
	for _, item := range items {
		info, infoErr := item.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("读取目录项 %s 失败：%w", item.Name(), infoErr)
		}
		entries = append(entries, files.Entry{
			Name:      item.Name(),
			Directory: item.IsDir(),
			Size:      info.Size(),
		})
	}
	return entries, nil
}

// resolveFile 把相对名字放到根目录下，拒绝绝对路径和字面上的跨根路径。
func (s *localStore) resolveFile(name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("路径 %s 必须相对于文件根目录", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径 %s 超出文件根目录", name)
	}
	return filepath.Join(s.root, clean), nil
}
