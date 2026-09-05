package commands

import "context"

// 数据。一条已登记命令的显示信息。
type Definition struct {
	Name        string
	Description string
}

// 契约。一条平台命令：按名调用，立刻执行。
type Command interface {
	Name() string
	Description() string
	Run(ctx context.Context, sessionID string) error
}

// 契约。命令登记处提供的操作。
type Commands interface {
	// 登记与查询
	Register(command Command) error
	Get(name string) (Command, error)
	List() []Definition

	// 按名执行
	Call(ctx context.Context, name, sessionID string) error
}
