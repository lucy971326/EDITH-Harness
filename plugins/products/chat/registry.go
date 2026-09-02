package chat

// 活对象。registry 汇集 Chat 对外提供的页面插槽登记处。
type registry struct {
	panels  *PanelRegistry
	actions *messageActionRegistry
}

func newRegistry() *registry {
	return &registry{panels: newPanelRegistry(), actions: newMessageActionRegistry()}
}

func (r *registry) RegisterPanel(panel Panel) error {
	return r.panels.RegisterPanel(panel)
}

func (r *registry) Panels() []PanelDefinition {
	return r.panels.Panels()
}

func (r *registry) Panel(id string) (Panel, bool) {
	return r.panels.Panel(id)
}

func (r *registry) RegisterMessageAction(action MessageAction) error {
	return r.actions.RegisterMessageAction(action)
}

func (r *registry) MessageActions() []MessageActionDefinition {
	return r.actions.MessageActions()
}

func (r *registry) MessageAction(id string) (MessageAction, bool) {
	return r.actions.MessageAction(id)
}
