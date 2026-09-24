package compact

import (
	"context"
	"harness/internal/commands"
	"harness/internal/runner"
)

// New 创建由 Runner 执行的压缩命令。
func New(r *runner.Runner) commands.Command { return command{runner: r} }

// 活对象。调用 Runner.Compact 的平台命令。
type command struct{ runner *runner.Runner }

func (command) Name() string        { return "compact" }
func (command) Description() string { return "压缩当前对话的有效历史" }
func (c command) Run(ctx context.Context, sessionID string) error {
	return c.runner.Compact(ctx, sessionID)
}
