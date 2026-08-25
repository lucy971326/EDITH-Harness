// Package projects 管理工作目录及其会话归属。
package projects

import "fmt"

// Project 是一个工作目录及其长期选择状态。
type Project struct {
	ID           string // 稳定身份，创建后不变
	Name         string // 给用户看的项目名称
	Root         string // 规范化后的工作目录，创建后不变
	LastPresetID string // 上次成功使用的 Agent 模式
	Archived     bool   // 归档后保留历史，不再作为默认可用项目
}

// Store 是项目的存取口；具体介质由持久化插件提供。
type Store interface {
	Create(project Project) error
	Get(id string) (Project, error)
	List() ([]Project, error)
	Update(project Project) error
}

// Validate 检查项目是否具备可持久化的基本信息。
func Validate(project Project) error {
	if project.ID == "" {
		return fmt.Errorf("项目必须有 id")
	}
	if project.Name == "" {
		return fmt.Errorf("项目 %s 必须有名称", project.ID)
	}
	if project.Root == "" {
		return fmt.Errorf("项目 %s 必须有根目录", project.ID)
	}
	return nil
}
