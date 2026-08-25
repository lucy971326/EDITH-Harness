// Package environment 定义给单段会话安装执行能力的环境插座。
package environment

import "harness/core"

// Provider 把同一项目根目录的 files、process 和 shell 装进会话作用域。
type Provider interface {
	Mount(scope *core.App, root string) error
}

// Get 从 App 领取当前选中的执行环境。
func Get(app *core.App) (Provider, error) {
	return core.Resolve[Provider](app, "environment")
}
