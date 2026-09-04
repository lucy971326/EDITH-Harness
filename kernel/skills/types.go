// Package skills 定义 Skill 发现登记处的契约。
package skills

// 数据。Skill 可用的范围。
type Scope string

const (
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
