package web

import (
	"context"
	"errors"
)

// errDirectoryPickCancelled 表示用户关闭了系统目录选择框。
var errDirectoryPickCancelled = errors.New("未选择目录")

// directoryPicker 为 Web UI 选择一个本机工作目录。
type directoryPicker interface {
	Pick(ctx context.Context) (string, error)
}
