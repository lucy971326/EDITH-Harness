// Package webclient 提供构建后嵌入 Harness 的 React 页面。
package webclient

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

// Handler 返回只读的 React 静态页面处理器。
func Handler() (http.Handler, error) {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("web client assets: %w", err)
	}
	_, err = fs.Stat(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("web client is not built: run make build: %w", err)
	}
	return http.FileServerFS(root), nil
}
