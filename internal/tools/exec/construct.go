package exec

import (
	"harness/internal/approvals"
	"harness/internal/machine"
	"harness/internal/tools"
)

// Register 将命令执行和持续输入工具登记到同一目录。
func Register(registry tools.Tools, processes machine.AgentProcesses, paths machine.Machine, approval *approvals.Service) error {
	for _, tool := range toolEntries(processes, paths, approval) {
		err := registry.Register(tool)
		if err != nil {
			return err
		}
	}
	return nil
}
