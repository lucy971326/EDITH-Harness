package subagents

import (
	delegation "harness/internal/subagents"
	"harness/internal/tools"
)

// Register 将委派工具登记到目录，不创建额外运行资源。
func Register(registry tools.Tools, service *delegation.Subagents) error {
	for _, tool := range toolEntries(service) {
		err := registry.Register(tool)
		if err != nil {
			return err
		}
	}
	return nil
}
