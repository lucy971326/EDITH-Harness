package tools

import "path/filepath"

// ResolvePath 按工具的路径规则得到要交给 machine 的路径。
func ResolvePath(workspace string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workspace, path)
}
