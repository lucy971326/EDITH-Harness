// Package agents 管理长期 Agent 档案和它们正在运行的会话。
package agents

import "fmt"

// AgentProfile 是一个长期 Agent 的可保存档案；工具名只指向已安装的工具代码。
type AgentProfile struct {
	ID           string   // 长期 Agent 的身份
	Revision     int      // 每次更新递增，供会话封面留历史标记
	Model        string   // 后续请求使用的模型
	SystemPrompt string   // 后续请求使用的系统提示词
	Tools        []string // 允许调用的全局工具名
	Archived     bool     // 归档后不再开新会话
}

// ProfileStore 是 Agent 档案的存取口；具体介质由持久化插件提供。
type ProfileStore interface {
	Create(profile AgentProfile) error
	Update(profile AgentProfile) error
	Get(id string) (AgentProfile, error)
	List() ([]AgentProfile, error)
	Archive(id string) error
}

// ValidateProfile 检查档案自身能不能保存；工具是否存在由 tools.Registry 检查。
func ValidateProfile(profile AgentProfile) error {
	if profile.ID == "" {
		return fmt.Errorf("agent 档案必须有 id")
	}
	if profile.Revision < 1 {
		return fmt.Errorf("agent %s 的档案版本必须从 1 起", profile.ID)
	}
	seen := make(map[string]bool)
	for _, name := range profile.Tools {
		if name == "" {
			return fmt.Errorf("agent %s 的工具名不能为空", profile.ID)
		}
		if seen[name] {
			return fmt.Errorf("agent %s 的工具 %s 写了两次", profile.ID, name)
		}
		seen[name] = true
	}
	return nil
}

func cloneProfile(profile AgentProfile) AgentProfile {
	copied := profile
	copied.Tools = append([]string(nil), profile.Tools...)
	return copied
}
