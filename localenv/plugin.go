// Package localenv 把同一个本地目录组装成 files、process 和 shell 三种能力。
package localenv

import (
	"harness/core"
	"harness/workspace/shell"
)

// Plugin 组合一套同根目录的本地执行环境。
type Plugin struct {
	Root        string // files 和 process 共用的工作目录
	OutputLimit int    // shell 的 stdout/stderr 各自保留上限；<=0 用默认值
}

func (Plugin) Name() string { return "localenv" }

// Start 一次登记 files、process、shell；任何一块失败都不留半套环境。
func (p Plugin) Start(app *core.App) error {
	fileStore, err := newFileStore(p.Root)
	if err != nil {
		return err
	}
	spawner, err := newSpawner(p.Root)
	if err != nil {
		return err
	}
	runner, err := shell.New(spawner, p.OutputLimit)
	if err != nil {
		_ = spawner.Close()
		return err
	}

	app.RegisterService("files", fileStore)
	app.RegisterService("process", spawner)
	app.RegisterService("shell", runner)
	app.OnCleanup(func() {
		_ = spawner.Close()
	})
	return nil
}
