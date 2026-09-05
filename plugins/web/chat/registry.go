package chat

// 活对象。registry 汇集 Chat 对外提供的页面插槽登记处。
type registry struct {
	panels          *PanelRegistry
	docks           *DockRegistry
	actions         *messageActionRegistry
	composerActions *composerActionRegistry
	suggestions     *suggestionRegistry
}

func newRegistry() *registry {
	return &registry{
		panels:          newPanelRegistry(),
		docks:           newDockRegistry(),
		actions:         newMessageActionRegistry(),
		composerActions: newComposerActionRegistry(),
		suggestions:     newSuggestionRegistry(),
	}
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

func (r *registry) RegisterDock(dock Dock) error {
	return r.docks.RegisterDock(dock)
}

func (r *registry) Docks() []DockDefinition {
	return r.docks.Docks()
}

func (r *registry) Dock(id string) (Dock, bool) {
	return r.docks.Dock(id)
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

func (r *registry) RegisterComposerAction(action ComposerAction) error {
	return r.composerActions.RegisterComposerAction(action)
}

func (r *registry) ComposerActions() []ComposerActionDefinition {
	return r.composerActions.ComposerActions()
}

func (r *registry) ComposerAction(id string) (ComposerAction, bool) {
	return r.composerActions.ComposerAction(id)
}

func (r *registry) RegisterSuggestionSource(source SuggestionSource) error {
	return r.suggestions.Register(source)
}

func (r *registry) Suggestions(prefix string, context SuggestionContext) ([]Suggestion, error) {
	return r.suggestions.List(prefix, context)
}
