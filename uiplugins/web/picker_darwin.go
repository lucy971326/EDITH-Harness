//go:build darwin

package web

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// nativeDirectoryPicker 通过 macOS 的原生对话框选择一个目录。
type nativeDirectoryPicker struct{}

// newNativeDirectoryPicker 返回当前系统的目录选择器。
func newNativeDirectoryPicker() directoryPicker {
	return nativeDirectoryPicker{}
}

// Pick 打开系统目录选择框，返回选中的绝对路径。
func (nativeDirectoryPicker) Pick(ctx context.Context) (string, error) {
	const script = `try
POSIX path of (choose folder with prompt "选择项目工作目录")
on error number -128
return ""
end try`
	command := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("打开系统目录选择框失败：%w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errDirectoryPickCancelled
	}
	return root, nil
}
