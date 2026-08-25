// Package presetjson 提供把 Agent 模式各版本保存为 JSON 文件的持久化插件。
package presetjson

import (
	"harness/core"
	"harness/presets"
)

// Plugin 把 JSON 模式库作为全局能力装进 App。
type Plugin struct {
	Root string // 所有 Agent 模式版本所在目录
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "persistence-presetjson"
}

// Start 创建模式库并登记给 presets.Plugin 领取。
func (p Plugin) Start(app *core.App) error {
	store, err := New(p.Root)
	if err != nil {
		return err
	}
	app.RegisterService("preset-store", presets.Store(store))
	return nil
}
