// Package clientassets 提供构建后嵌入 Harness 的 React 页面。
package clientassets

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed dist
var assets embed.FS

// AssetsFS 返回同一份 React 构建产物，供 Web 与桌面资源服务使用。
func AssetsFS() (fs.FS, error) {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("client assets: %w", err)
	}
	_, err = fs.Stat(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("client is not built: run make build: %w", err)
	}
	return root, nil
}
