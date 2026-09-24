// Package workspacepicker 提供本机目录选择，不参与 Harness 产品业务。
package workspacepicker

import (
	"context"
	"errors"
)

// ErrCanceled 表示用户主动取消原生目录选择。
var ErrCanceled = errors.New("appserver: workspace selection canceled")

// 契约。Picker 是 Server 可注入的目录选择函数。
type Picker func(context.Context) (string, error)
