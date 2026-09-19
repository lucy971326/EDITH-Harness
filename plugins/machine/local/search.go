package machinelocal

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitignore "github.com/denormal/go-gitignore"

	"harness/kernel/machine"
)

// SearchPaths 按目录顺序搜索当前磁盘；每次查询独立，不维护第二份文件索引。
func (m *local) SearchPaths(ctx context.Context, workspace, query string) (machine.PathSearchResult, error) {
	result := machine.PathSearchResult{Entries: []machine.PathMatch{}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	workspace = filepath.Clean(workspace)
	query = strings.ToLower(strings.ReplaceAll(query, "\\", "/"))
	if !filepath.IsAbs(workspace) {
		return result, fmt.Errorf("machine-local: search requires an absolute workspace")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("machine-local: search workspace is not a directory")
	}

	// 只保留当前递归路径上的规则；离开目录时还原，不能把兄弟目录的规则串用。
	var walk func(string, []gitignore.GitIgnore) error
	walk = func(directory string, inherited []gitignore.GitIgnore) error {
		err := ctx.Err()
		if err != nil {
			return err
		}
		rules := inherited
		data, err := os.ReadFile(filepath.Join(directory, ".gitignore"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			rules = append(rules, gitignore.New(bytes.NewReader(data), directory, nil))
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			err = ctx.Err()
			if err != nil {
				return err
			}
			if entry.Name() == ".git" || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if !entry.IsDir() && !entry.Type().IsRegular() {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			ignored := false
			for index := len(rules) - 1; index >= 0; index-- {
				rulePath, err := filepath.Rel(rules[index].Base(), path)
				if err != nil {
					return err
				}
				match := rules[index].Relative(rulePath, entry.IsDir())
				if match != nil {
					ignored = match.Ignore()
					break
				}
			}
			if ignored {
				continue
			}
			relative, err := filepath.Rel(workspace, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if query == "" || strings.Contains(strings.ToLower(relative), query) {
				if len(result.Entries) == 50 {
					result.Truncated = true
					return fs.SkipAll
				}
				kind := "file"
				if entry.IsDir() {
					kind = "directory"
				}
				result.Entries = append(result.Entries, machine.PathMatch{Path: relative, Kind: kind})
			}
			if entry.IsDir() && query != "" {
				err = walk(path, rules)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	err = walk(workspace, nil)
	if err != nil && err != fs.SkipAll {
		return machine.PathSearchResult{}, fmt.Errorf("machine-local: search paths: %w", err)
	}
	return result, nil
}
