// Package files 定义工作空间的文件能力。
package files

import (
	"context"

	"harness/core"
)

const serviceName = "files"

// Entry 是列目录时返回的一项。
type Entry struct {
	Name      string // 这一项的文件名
	Directory bool   // true 表示目录
	Size      int64  // 字节数
}

// Store 在同一个根目录下读、写、列文件。
type Store interface {
	Read(ctx context.Context, name string) ([]byte, error)
	Write(ctx context.Context, name string, data []byte) error
	List(ctx context.Context, name string) ([]Entry, error)
}

// Get 从 App 取出 files 能力。
func Get(app *core.App) (Store, error) {
	return core.Resolve[Store](app, serviceName)
}
