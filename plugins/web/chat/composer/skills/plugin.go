// Package skills 将 Agent 当前可用的 Skill 填入 Chat 的输入候选登记处。
package skills

import (
	"fmt"
	"path/filepath"
	"strings"

	chatservice "harness/kernel/chat"
	"harness/kernel/host"
	kernskills "harness/kernel/skills"
	chat "harness/plugins/web/chat"
	"harness/surface/web/ui"
)

// 活对象。把 Skill 候选来源填入 Chat 的插件。
type Plugin struct {
	source *source
}

// New 造 Skill 输入候选插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "chat-composer-skills" }

func (p *Plugin) Start(h *host.Host) error {
	business, err := host.Resolve[*chatservice.Service](h, "chatService")
	if err != nil {
		return fmt.Errorf("chat-composer-skills: resolve chat service: %w", err)
	}
	chatService, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return fmt.Errorf("chat-composer-skills: resolve chat: %w", err)
	}
	p.source = &source{business: business}
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

// 活对象。按当前工作区查询 Skill 候选的来源。
type source struct {
	business *chatservice.Service
}

func (s *source) ID() string { return "skills" }

func (s *source) Prefixes() []string { return []string{"/", "$"} }

func (s *source) List(context chat.SuggestionContext) ([]chat.Suggestion, error) {
	if s == nil || s.business == nil {
		return nil, fmt.Errorf("chat-composer-skills: source is not ready")
	}
	available, err := s.business.AvailableSkills(context.Workspace)
	if err != nil {
		return nil, err
	}
	items := make([]chat.Suggestion, 0, len(available))
	for _, skill := range available {
		items = append(items, chat.Suggestion{
			Name:        skill.Name,
			Description: skill.Description,
			Icon:        ui.IconSkill,
			Scope:       scopeLabel(skill.Scope, context.Workspace),
		})
	}
	return items, nil
}

func scopeLabel(scope kernskills.Scope, workspace string) string {
	switch scope {
	case kernskills.ScopeSystem:
		return "系统"
	case kernskills.ScopeUser:
		return "个人"
	case kernskills.ScopeWorkspace:
		name := filepath.Base(filepath.Clean(strings.TrimSpace(workspace)))
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
		return "项目"
	default:
		return ""
	}
}
