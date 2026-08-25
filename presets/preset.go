// Package presets 管理可版本化的 Agent 模式。
package presets

import "fmt"

// Revision 是一个 Agent 模式的不可变版本；工具名只指向已安装的工具代码。
type Revision struct {
	ID           string   // Agent 模式的稳定身份
	Revision     int      // 此身份下不可变版本号，从 1 起递增
	SystemPrompt string   // 后续请求使用的系统提示词
	Tools        []string // 允许调用的全局工具名
	Archived     bool     // 归档后不能用于新会话
}

// Preset 是模式当前版本的完整内容。
type Preset = Revision

// Validate 检查一个模式版本能否保存；工具是否存在由 tools.Registry 检查。
func Validate(revision Revision) error {
	if revision.ID == "" {
		return fmt.Errorf("Agent 模式必须有 id")
	}
	if revision.Revision < 1 {
		return fmt.Errorf("Agent 模式 %s 的版本必须从 1 起", revision.ID)
	}
	seen := make(map[string]bool)
	for _, name := range revision.Tools {
		if name == "" {
			return fmt.Errorf("Agent 模式 %s 的工具名不能为空", revision.ID)
		}
		if seen[name] {
			return fmt.Errorf("Agent 模式 %s 的工具 %s 写了两次", revision.ID, name)
		}
		seen[name] = true
	}
	return nil
}

func clone(revision Revision) Revision {
	copied := revision
	copied.Tools = append([]string(nil), revision.Tools...)
	return copied
}
