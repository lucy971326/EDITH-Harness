// Package localenv 把同一个本地目录组装成 files、process 和 shell 三种能力。
package localenv

import (
	"harness/core"
	"harness/environment"
	"harness/workspace/shell"
)

// Plugin 登记按项目根目录创建本地执行环境的能力。
type Plugin struct {
	OutputLimit int // shell 的 stdout/stderr 各自保留上限；<=0 用默认值
}

func (Plugin) Name() string { return "localenv" }

// Start 登记本地环境提供者；真正的执行能力在会话启动时挂载。
func (p Plugin) Start(app *core.App) error {
	app.RegisterService("environment", environment.Provider(&provider{outputLimit: p.OutputLimit}))
	return nil
}

type provider struct {
	outputLimit int
}

// Mount 给一个会话挂载同根的 files、process 和 shell；作用域关闭时收掉进程树。
func (p *provider) Mount(scope *core.App, root string) error {
	fileStore, err := newFileStore(root)
	if err != nil {
		return err
	}
	spawner, err := newSpawner(root)
	if err != nil {
		return err
	}
	runner, err := shell.New(spawner, p.outputLimit)
	if err != nil {
		_ = spawner.Close()
		return err
	}

	scope.RegisterService("files", fileStore)
	scope.RegisterService("process", spawner)
	scope.RegisterService("shell", runner)
	scope.OnCleanup(func() {
		_ = spawner.Close()
	})
	return nil
}
