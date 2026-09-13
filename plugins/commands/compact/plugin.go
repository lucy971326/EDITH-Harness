// Package compact 把 compact 命令填入 commands 登记处。
package compact

import (
	"context"
	"fmt"

	"harness/kernel/commands"
	"harness/kernel/host"
	"harness/kernel/runner"
)

// 活对象。把 compact 命令填入命令登记处的插件。
type Plugin struct {
	runner *runner.Runner
}

// New 造 compact 命令插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "commands-compact" }

func (p *Plugin) Start(h *host.Host) error {
	registry, err := host.Resolve[commands.Commands](h, "commands")
	if err != nil {
		return fmt.Errorf("commands-compact: resolve commands: %w", err)
	}
	p.runner, err = host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		return fmt.Errorf("commands-compact: resolve runner: %w", err)
	}
	err = registry.Register(command{runner: p.runner})
	if err != nil {
		p.runner = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.runner = nil
	return nil
}

// 活对象。调用 Runner.Compact 的平台命令。
type command struct {
	runner *runner.Runner
}

func (command) Name() string { return "compact" }

func (command) Description() string { return "压缩当前对话的有效历史" }

func (c command) Run(ctx context.Context, sessionID string) error {
	return c.runner.Compact(ctx, sessionID)
}
