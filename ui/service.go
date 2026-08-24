// Package ui 定义当前选中界面的启动入口。
package ui

import (
	"context"

	"harness/core"
)

// Service 是一个已装好界面的启动入口。
type Service interface {
	Run(ctx context.Context) error
}

// Get 从 App 取当前选中的界面。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "ui")
}
