// Package machine 定义当前机器服务的契约。
package machine

import (
	"context"
)

// 数据。当前机器目录中的一个直接子项。
type DirEntry struct {
	Name  string
	IsDir bool
}

// 契约。文件和进程所在机器提供的操作。
type Machine interface {
	// HomeDir 返回当前机器上用户的主目录。
	HomeDir() (string, error)

	// 文件操作
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]DirEntry, error)
	WriteFile(path string, data []byte) error

	// 路径与进程
	ResolvePath(workspace string, path string) string
	Run(ctx context.Context, dir string, argv []string) (stdout, stderr []byte, err error)
}
