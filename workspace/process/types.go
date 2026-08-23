// Package process 定义工作空间的长期进程能力。
package process

import (
	"context"
	"io"
	"os"

	"harness/core"
)

const serviceName = "process"

// Command 说明要启动的程序、参数和工作目录。
type Command struct {
	Program string   // 程序名或可执行文件路径
	Args    []string // 程序参数
	Dir     string   // 相对于执行环境根目录的工作目录
	Env     []string // 追加的 KEY=value 环境变量
}

// Exit 是进程结束时的状态；被信号杀死时 Code 为 -1。
type Exit struct {
	Code int // 0 表示成功，-1 表示被信号结束
}

// Handle 是一个已启动进程的输入、输出和生死控制口。
type Handle interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader
	Signal(signal os.Signal) error
	Wait() (Exit, error)
	Close() error
}

// Spawner 启动长期进程，并在 Close 时收掉它启动的所有进程树。
type Spawner interface {
	Spawn(ctx context.Context, command Command) (Handle, error)
	Close() error
}

// Get 从 App 取出 process 能力。
func Get(app *core.App) (Spawner, error) {
	return core.Resolve[Spawner](app, serviceName)
}
