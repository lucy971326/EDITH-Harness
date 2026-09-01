package chat

import "errors"

// ErrCanceled 表示用户主动取消原生目录选择。
var ErrCanceled = errors.New("chat: workspace selection canceled")
