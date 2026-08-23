// Package profilejson 提供把长期 Agent 档案保存为 JSON 文件的持久化插件。
package profilejson

import (
	"harness/core"
)

// Plugin 把 JSON 档案库作为全局能力装进 App。
type Plugin struct {
	Root string // 所有 Agent 档案文件所在目录
}

func (Plugin) Name() string { return "persistence-profilejson" }

// Start 创建档案库并登记给 loop.Plugin 领取。
func (p Plugin) Start(app *core.App) error {
	store, err := New(p.Root)
	if err != nil {
		return err
	}
	app.RegisterService("agent-profiles", store)
	return nil
}
