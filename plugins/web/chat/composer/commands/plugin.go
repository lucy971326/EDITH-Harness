// Package commands 将平台命令填入 Chat 的输入候选登记处。
package commands

import (
	"fmt"

	"harness/kernel/commands"
	"harness/kernel/host"
	chat "harness/plugins/web/chat"
	"harness/surface/web/ui"
)

// 活对象。把命令候选来源填入 Chat 的插件。
type Plugin struct {
	source *source
}

// New 造命令输入候选插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "chat-composer-commands" }

func (p *Plugin) Start(h *host.Host) error {
	registry, err := host.Resolve[commands.Commands](h, "commands")
	if err != nil {
		return fmt.Errorf("chat-composer-commands: resolve commands: %w", err)
	}
	chatService, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return fmt.Errorf("chat-composer-commands: resolve chat: %w", err)
	}
	p.source = &source{commands: registry}
	if err := chatService.RegisterSuggestionSource(p.source); err != nil {
		p.source = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.source = nil
	return nil
}

// 活对象。按当前命令名单生成 / 候选。
type source struct {
	commands commands.Commands
}

func (s *source) ID() string { return "commands" }

func (s *source) Prefixes() []string { return []string{"/"} }

func (s *source) List(chat.SuggestionContext) ([]chat.Suggestion, error) {
	if s == nil || s.commands == nil {
		return nil, fmt.Errorf("chat-composer-commands: source is not ready")
	}
	definitions := s.commands.List()
	items := make([]chat.Suggestion, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, chat.Suggestion{
			Name:        definition.Name,
			Description: definition.Description,
			Icon:        ui.IconCompact,
		})
	}
	return items, nil
}
