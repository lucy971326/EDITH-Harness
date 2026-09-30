package commands

import "context"

// 数据。一条已登记命令的显示信息。
type Definition struct {
	ID           string
	Kind         string
	Name         string
	Description  string
	ArgumentHint string
	Scope        string
	Source       string
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
	Prompts() *PromptStore
}

// 数据。一条本机提示词命令；ID 由来源和名称确定，不写入配置文件。
type Prompt struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ArgumentHint string `json:"argumentHint,omitempty"`
	Prompt       string `json:"prompt"`
	Scope        string `json:"scope,omitempty"`
	Source       string `json:"source,omitempty"`
}

// 数据。一个作用域的命令及配置文件版本。
type PromptFile struct {
	Commands []Prompt `json:"commands"`
	Hash     string   `json:"hash"`
}
