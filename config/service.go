// Package config 是配置插件：普通配置进抽屉柜，钥匙进保险柜。
package config

import "harness/core"

// Settings 是抽屉柜：插件登记自己的抽屉，只读写自己那一格。
type Settings interface {
	Register(drawer Drawer) (func(), error)
	Get(name string) (map[string]string, error)
	Set(name string, values map[string]string) error
}

// Credentials 是保险柜：明文只交给调用方；问「有没有」不给值。
type Credentials interface {
	Register(need Need) (func(), error)
	Resolve(drawer string, key string) (string, error)
	Configured(drawer string, key string) bool
	Set(drawer string, key string, value string) error
}

// GetSettings 从场地取抽屉柜。
func GetSettings(app *core.App) (Settings, error) {
	return core.Resolve[Settings](app, "settings")
}

// GetCredentials 从场地取保险柜。
func GetCredentials(app *core.App) (Credentials, error) {
	return core.Resolve[Credentials](app, "credentials")
}
