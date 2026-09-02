package chat

import "github.com/a-h/templ"

// 数据。一个已登记面板类型的显示与默认打开信息。
type PanelDefinition struct {
	ID                 string
	Name               string
	Icon               string
	Order              int
	DefaultInstanceKey string
	DefaultTabTitle    string
}

// 数据。浏览器当前打开的一个面板实例。
type PanelTab struct {
	Type        string
	InstanceKey string
}

// 数据。渲染面板所需的当前会话事实。
type PanelContext struct {
	SessionID string
	Workspace string
}

// 契约。一个可填入 Chat 右侧面板的类型。
type Panel interface {
	Definition() PanelDefinition
	Render(PanelContext, PanelTab) (templ.Component, error)
}

// 契约。Chat 提供给面板插件的右侧面板登记处。
type Service interface {
	RegisterPanel(Panel) error
	Panels() []PanelDefinition
	Panel(id string) (Panel, bool)
}
