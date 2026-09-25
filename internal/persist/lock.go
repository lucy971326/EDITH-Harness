package persist

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockRoot 独占用户数据目录，防止两个后台分别缓存并改写同一份数据。
// 锁文件保留在目录中；操作系统在进程退出时释放文件锁。
func LockRoot(root string) (*os.File, error) {
	err := os.MkdirAll(root, 0o700)
	if err != nil {
		return nil, fmt.Errorf("persist: create data directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("persist: open data lock: %w", err)
	}
	err = lockFile(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("persist: data directory is in use: %w", err)
	}
	return file, nil
}
