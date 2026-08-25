package projects

import (
	"fmt"

	"harness/core"
	"harness/session"
)

// Plugin 把项目服务装进 App。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "projects"
}

// Start 领取项目存储和账本管家，再登记项目服务。
func (Plugin) Start(app *core.App) error {
	store, err := core.Resolve[Store](app, "project-store")
	if err != nil {
		return fmt.Errorf("projects 要先有项目存储（project-store）：%w", err)
	}
	books, err := session.Get(app)
	if err != nil {
		return fmt.Errorf("projects 要先有账本管家（sessions）：%w", err)
	}
	app.RegisterService("projects", New(store, books))
	return nil
}

// Get 从 App 取项目服务。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "projects")
}
