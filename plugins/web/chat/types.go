package chat

import (
	"context"

	"github.com/a-h/templ"

	"harness/kernel/session"
	"harness/surface/web/ui"
)

// 数据。一个已登记面板类型的显示与默认打开信息。
type PanelDefinition struct {
	ID                 string
	Name               string
	Icon               ui.IconName
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

// 数据。一个已登记 Dock 条目的静态身份。
type DockDefinition struct {
	ID    string
	Name  string
	Order int
}

// 数据。渲染 Dock 所需的当前会话事实。
type DockContext struct {
	SessionID string
	Workspace string
}

// 契约。一个可填入 Chat 输入框上方的持续状态条目。
type Dock interface {
	Definition() DockDefinition
	Render(DockContext) (templ.Component, error)
}

// 数据。一个 Dock 条目的状态已经变化。
type DockChanged struct {
	SessionID string
	DockID    string
}

// 数据。一张消息卡片的类型。
type MessageCardType string

const (
	MessageCardUser      MessageCardType = "user"
	MessageCardAssistant MessageCardType = "assistant"
)

// 数据。一个已登记消息动作的显示与适用卡片类型。
type MessageActionDefinition struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Icon    string            `json:"icon"`
	Order   int               `json:"order"`
	Targets []MessageCardType `json:"targets"`
}

// 数据。浏览器提交给消息动作的一张耐久消息卡片身份。
type MessageActionTarget struct {
	CardType        MessageCardType
	EntryID         string
	RunID           string
	BoundaryEntryID string
}

// 数据。Chat 已验证后交给消息动作的当前分叉事实。
type MessageActionContext struct {
	SessionID string
	Entries   []session.Entry
	Target    MessageActionTarget
}

// 数据。一项消息动作成功后的浏览器回包。
type MessageActionResult struct {
	Text     string `json:"text"`
	Redirect string `json:"redirect"`
}

// 契约。一个可填入 Chat 消息卡片下方的标准动作。
type MessageAction interface {
	Definition() MessageActionDefinition
	Execute(context.Context, MessageActionContext) (MessageActionResult, error)
}

// 数据。一个已登记输入工具栏动作的静态身份。
type ComposerActionDefinition struct {
	ID    string
	Order int
}

// 数据。渲染输入工具栏动作所需的当前会话事实。
type ComposerActionContext struct {
	SessionID string
	Workspace string
}

// 契约。一个可填入 Chat 输入工具栏的扩展动作。
type ComposerAction interface {
	Definition() ComposerActionDefinition
	Render(ComposerActionContext) (templ.Component, error)
}

// 数据。输入候选的显示信息。
type Suggestion struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Icon        ui.IconName `json:"icon"`
	Scope       string      `json:"scope"`
	SourceID    string      `json:"sourceID"`
}

// 数据。输入候选查询所需的当前 Agent 和工作区。
type SuggestionContext struct {
	AgentID   string
	Workspace string
}

// 契约。一个可填入 Chat composer.suggestions 登记处的候选来源。
type SuggestionSource interface {
	ID() string

	Prefixes() []string
	List(SuggestionContext) ([]Suggestion, error)
}

// 契约。Chat 提供给页面插槽插件的登记处。
type Service interface {
	// 右侧面板插槽 (Sidepanel)
	RegisterPanel(Panel) error
	Panels() []PanelDefinition
	Panel(id string) (Panel, bool)

	// 持续状态插槽 (Dock)
	RegisterDock(Dock) error
	Docks() []DockDefinition
	Dock(id string) (Dock, bool)

	// 消息动作插槽 (Message Actions)
	RegisterMessageAction(MessageAction) error
	MessageActions() []MessageActionDefinition
	MessageAction(id string) (MessageAction, bool)

	// 输入工具栏插槽 (Composer Actions)
	RegisterComposerAction(ComposerAction) error
	ComposerActions() []ComposerActionDefinition
	ComposerAction(id string) (ComposerAction, bool)

	// 输入候选登记处 (Composer Suggestions)
	RegisterSuggestionSource(SuggestionSource) error
	Suggestions(prefix string, context SuggestionContext) ([]Suggestion, error)
}
