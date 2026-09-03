package demo

import (
	"github.com/a-h/templ"

	"harness/plugins/products/chat"
)

type composerAction struct{}

func (composerAction) Definition() chat.ComposerActionDefinition {
	return chat.ComposerActionDefinition{
		ID:    "demo",
		Order: 10,
	}
}

func (composerAction) Render(chat.ComposerActionContext) (templ.Component, error) {
	return content(), nil
}
