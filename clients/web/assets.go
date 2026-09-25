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

// AssetsFS 返回同一份 React 构建产物，供 Web 与桌面资源服务使用。
func AssetsFS() (fs.FS, error) {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("web client assets: %w", err)
	}
	_, err = fs.Stat(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("web client is not built: run make build: %w", err)
	}
	return root, nil
}

// Handler 返回只读的 React 静态页面处理器。
func Handler() (http.Handler, error) {
	root, err := AssetsFS()
	if err != nil {
		return nil, err
	}
	return http.FileServerFS(root), nil
}
