// Package skills 定义 Skill 发现登记处的契约。
package skills

import "errors"

// 数据。Skill 可用的范围。
type Scope string

const (
	ScopeSystem    Scope = "system"
	ScopeUser      Scope = "user"
	ScopeWorkspace Scope = "workspace"
)

// 数据。一条可供 Agent 使用的 Skill 摘要。
type Skill struct {
	Name        string
	Description string
	Location    string
	Scope       Scope
}

// 契约。Skill 来源负责按本轮工作区发现 Skill。
type Provider interface {
	Name() string
	List(workspace string) ([]Skill, error)
}

// 契约。Skill 发现登记处提供 Provider 注册与动态查询。
type Skills interface {
	// Provider 管理
	Register(provider Provider) error

	// Skill 查询
	List(workspace string) ([]Skill, error)
}

var (
	ErrInvalid  = errors.New("invalid skill")
	ErrConflict = errors.New("skill changed")
	ErrNotFound = errors.New("skill not found")
)

// 数据。设置页中的一个来源条目，损坏的 Skill 也保留在清单中。
type SettingsItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Overridden  bool   `json:"overridden"`
	Error       string `json:"error,omitempty"`
}

// 数据。设置页读取的一份 Skill 正文和文件版本。
type Document struct {
	Content   string   `json:"content"`
	Version   string   `json:"version"`
	Resources []string `json:"resources"`
}

// 契约。Skill 设置只管理个人 Harness 来源。
type Settings interface {
	Catalog(workspace string) ([]SettingsItem, error)
	ReadDocument(workspace, source, name string) (Document, error)
	Save(name, content, version string, create bool) (Document, error)
	SetEnabled(name string, enabled bool) error
	Delete(name, version string) error
}
