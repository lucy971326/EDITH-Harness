// Package machine 定义当前机器服务的契约。
package machine

import "context"

// 契约。文件和进程所在机器提供的操作。
type Machine interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Run(ctx context.Context, dir string, argv []string) (stdout, stderr []byte, err error)
}
