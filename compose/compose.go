// Package compose 读取 edith-harness 配置并按固定顺序组装插件。
package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"harness/core"
)

// Runtime 是一次已经组装好的 edith-harness 运行时。
type Runtime struct {
	App  *core.App // 已按依赖顺序装好的公共场地
	Home string    // 用户私有目录
}

// Open 读取用户配置并组装完整运行时。首次调用会写无密钥模板后返回错误。
func Open(home string) (*Runtime, error) {
	absoluteHome, err := privateDirectory(home, "用户目录")
	if err != nil {
		return nil, err
	}
	config, err := loadConfig(absoluteHome)
	if err != nil {
		return nil, err
	}
	plugins, err := selectPlugins(config, absoluteHome)
	if err != nil {
		return nil, err
	}
	app := core.New()
	err = app.Install(plugins.ordered()...)
	if err != nil {
		app.Close()
		return nil, err
	}
	return &Runtime{App: app, Home: absoluteHome}, nil
}

func privateDirectory(path string, label string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s不能为空", label)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析%s失败：%w", label, err)
	}
	err = os.MkdirAll(absolute, 0o700)
	if err != nil {
		return "", fmt.Errorf("创建%s失败：%w", label, err)
	}
	err = os.Chmod(absolute, 0o700)
	if err != nil {
		return "", fmt.Errorf("收紧%s权限失败：%w", label, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("打开%s失败：%w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s路径 %s 不是目录", label, absolute)
	}
	return absolute, nil
}
