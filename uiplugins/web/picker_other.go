//go:build !darwin

package web

import (
	"context"
	"fmt"
)

// unavailableDirectoryPicker 在尚未实现原生选择器的平台给出明确错误。
type unavailableDirectoryPicker struct{}

// newNativeDirectoryPicker 返回当前系统的目录选择器。
func newNativeDirectoryPicker() directoryPicker {
	return unavailableDirectoryPicker{}
}

// Pick 返回当前平台尚未支持原生目录选择的错误。
func (unavailableDirectoryPicker) Pick(context.Context) (string, error) {
	return "", fmt.Errorf("当前系统尚未支持原生目录选择")
}
