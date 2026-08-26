// Package commands 是人类命令登记处：登记、解析、把斜杠行分发给命令。
package commands

import (
	"context"

	"harness/core"
)

// Service 是其他插件登记和调用人类命令的公共入口。
type Service interface {
	Register(command Command) (func(), error)
	List() []Descriptor
	Find(name string) (Command, bool)
	Execute(ctx context.Context, sessionID string, line string) (Result, error)
}

// Get 从场地取命令登记处。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "commands")
}
