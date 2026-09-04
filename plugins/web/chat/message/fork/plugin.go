// Package fork 为 Chat 已完成回答提供创建独立分叉会话的消息动作。
package fork

import (
	"context"
	"fmt"

	"harness/kernel/host"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/plugins/web/chat"
)

// 活对象。Plugin 将分叉动作填入 Chat 的 message.actions 插槽。
type Plugin struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
}

// New 创建分叉消息动作插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "message-fork" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return fmt.Errorf("message fork: resolve chat: %w", err)
	}
	p.sessions, err = host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		return fmt.Errorf("message fork: resolve sessions: %w", err)
	}
	p.settings, err = host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		return fmt.Errorf("message fork: resolve session settings: %w", err)
	}
	err = service.RegisterMessageAction(action{sessions: p.sessions, settings: p.settings})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.sessions = nil
	p.settings = nil
	return nil
}

// 活对象。action 从已完成的助手回答创建一场独立会话。
type action struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
}

func (action) Definition() chat.MessageActionDefinition {
	return chat.MessageActionDefinition{
		ID:      "fork",
		Name:    "分叉",
		Icon:    "⑂",
		Order:   20,
		Targets: []chat.MessageCardType{chat.MessageCardAssistant},
	}
}

func (a action) Execute(_ context.Context, input chat.MessageActionContext) (chat.MessageActionResult, error) {
	throughEntryID, err := assistantSegmentEnd(input.Entries, input.Target)
	if err != nil {
		return chat.MessageActionResult{}, err
	}
	setup, err := a.settings.For(input.SessionID)
	if err != nil {
		return chat.MessageActionResult{}, fmt.Errorf("message fork: load session settings: %w", err)
	}
	title, err := forkTitle(a.sessions, input.SessionID)
	if err != nil {
		return chat.MessageActionResult{}, err
	}
	destinationID, err := session.NewID()
	if err != nil {
		return chat.MessageActionResult{}, err
	}
	// 先写旁路设置；若后续账本复制失败，它没有元数据，不会作为会话出现。
	err = a.settings.Put(destinationID, setup)
	if err != nil {
		return chat.MessageActionResult{}, fmt.Errorf("message fork: copy session settings: %w", err)
	}
	_, err = a.sessions.Fork(input.SessionID, destinationID, throughEntryID, title)
	if err != nil {
		return chat.MessageActionResult{}, fmt.Errorf("message fork: copy session: %w", err)
	}
	return chat.MessageActionResult{Redirect: "/chat/" + destinationID}, nil
}

func assistantSegmentEnd(entries []session.Entry, target chat.MessageActionTarget) (string, error) {
	start := -1
	for index, entry := range entries {
		if entry.ID == target.BoundaryEntryID && entry.Message.RunID == target.RunID && entry.Message.Role == session.RoleUser {
			start = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("message fork: assistant segment not found")
	}
	end := ""
	for _, entry := range entries[start+1:] {
		if entry.Message.RunID != target.RunID {
			continue
		}
		if entry.Message.Role == session.RoleUser {
			break
		}
		end = entry.ID
	}
	if end == "" {
		return "", fmt.Errorf("message fork: assistant segment has no durable result")
	}
	return end, nil
}

func forkTitle(store *session.Store, sessionID string) (string, error) {
	metas, err := store.List()
	if err != nil {
		return "", fmt.Errorf("message fork: list sessions: %w", err)
	}
	for _, meta := range metas {
		if meta.ID == sessionID {
			return meta.Title + " · 分叉", nil
		}
	}
	return "", fmt.Errorf("message fork: source session not found")
}
