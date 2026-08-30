// Package skills 定义 Skill 摘要登记处的契约。
package skills

// 数据。一条给 Agent 系统提示词使用的 Skill 摘要。
type Skill struct {
	Name    string
	Summary string
}

// 契约。Skill 摘要登记处提供的操作。
type Skills interface {
	Register(skill Skill) (unregister func(), err error)
	Get(names []string) ([]Skill, error)
	List() []Skill
}
