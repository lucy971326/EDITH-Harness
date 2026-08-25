package projectjson

import (
	"harness/core"
	"harness/projects"
)

// Plugin 把 JSON 项目库作为全局能力装进 App。
type Plugin struct {
	Root string // 所有项目文件所在目录
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "persistence-projectjson"
}

// Start 创建项目库并登记给 projects.Plugin 领取。
func (p Plugin) Start(app *core.App) error {
	store, err := New(p.Root)
	if err != nil {
		return err
	}
	app.RegisterService("project-store", projects.Store(store))
	return nil
}
