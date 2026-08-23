package jsonl

import (
	"harness/core"
)

// Plugin 把 JSONL Journal 作为全局持久化能力装进 App。
type Plugin struct {
	Root string // 所有 JSONL 账本文件所在目录
}

func (Plugin) Name() string { return "persistence-jsonl" }

// Start 创建 JSONL Journal 并登记给 session.Plugin 领取。
func (p Plugin) Start(app *core.App) error {
	journal, err := New(p.Root)
	if err != nil {
		return err
	}
	app.RegisterService("journal", journal)
	return nil
}
