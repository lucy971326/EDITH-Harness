package chat

import (
	"context"

	"github.com/a-h/templ"

	"harness/kernel/session"
)

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
	Text string `json:"text"`
}

// 契约。一个可填入 Chat 消息卡片下方的标准动作。
type MessageAction interface {
	Definition() MessageActionDefinition
	Execute(context.Context, MessageActionContext) (MessageActionResult, error)
}

// 契约。Chat 提供给面板和消息动作插件的登记处。
type Service interface {
	RegisterPanel(Panel) error
	Panels() []PanelDefinition
	Panel(id string) (Panel, bool)
	RegisterMessageAction(MessageAction) error
	MessageActions() []MessageActionDefinition
	MessageAction(id string) (MessageAction, bool)
}
