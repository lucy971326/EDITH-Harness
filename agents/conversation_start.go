package agents

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
)

// StartSession 用项目和模式当前版本开一段新会话，返回自动生成身份的运行门面。
func (m *conversationManager) StartSession(input StartInput, seed ...session.Event) (Conversation, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, fmt.Errorf("会话标题不能为空")
	}
	project, err := m.projects.Get(input.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("打开项目 %s 失败：%w", input.ProjectID, err)
	}
	if project.Archived {
		return nil, fmt.Errorf("项目 %s 已归档，不能开新会话", project.ID)
	}
	preset, err := m.presets.Get(input.PresetID)
	if err != nil {
		return nil, fmt.Errorf("读取 Agent 模式 %s 失败：%w", input.PresetID, err)
	}
	if preset.Archived {
		return nil, fmt.Errorf("Agent 模式 %s 已归档，不能开新会话", preset.ID)
	}
	err = presets.Validate(preset)
	if err != nil {
		return nil, err
	}
	err = m.tools.CheckAllowed(preset.Tools)
	if err != nil {
		return nil, fmt.Errorf("Agent 模式 %s 的工具不完整：%w", preset.ID, err)
	}
	if input.Model.Provider == "" && input.Model.Model == "" && input.Model.Thinking == "" {
		input.Model, err = m.llm.DefaultSelection()
		if err != nil {
			return nil, err
		}
	}
	err = m.llm.Validate(input.Model)
	if err != nil {
		return nil, err
	}
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	runner, err := m.reserve(sessionID)
	if err != nil {
		return nil, err
	}
	header := session.Header{
		ID:             sessionID,
		Title:          input.Title,
		CreatedAt:      time.Now().UTC(),
		ProjectID:      project.ID,
		ProjectRoot:    project.Root,
		PresetID:       preset.ID,
		PresetRevision: preset.Revision,
	}
	conversation, err := m.openNew(runner, project, preset, input.Model, header, seed)
	if err != nil {
		m.cancelReservation(sessionID, nil)
		return nil, err
	}
	m.publish(sessionID, conversation)
	conversation.Start()
	return conversation, nil
}

func (m *conversationManager) openNew(runner Runner, project projects.Project, preset presets.Revision, model llm.Selection, header session.Header, seed []session.Event) (PreparedConversation, error) {
	scope, err := m.mountScope(header.ID, header.ProjectRoot, preset.Tools)
	if err != nil {
		return nil, err
	}
	book, err := m.books.Create(header, seed...)
	if err != nil {
		m.cancelReservation(header.ID, scope)
		return nil, err
	}
	_, err = book.RecordModelSelection(session.ModelSelectedData{Provider: model.Provider, Model: model.Model, Thinking: model.Thinking})
	if err != nil {
		_ = m.books.Release(header.ID)
		m.cancelReservation(header.ID, scope)
		return nil, fmt.Errorf("会话 %s 记初始模型失败：%w", header.ID, err)
	}
	conversation, err := m.prepare(runner, project, preset, model, scope, book, len(seed) > 0)
	if err != nil {
		_ = m.books.Release(header.ID)
		m.cancelReservation(header.ID, scope)
		return nil, fmt.Errorf("会话 %s 准备失败：%w", header.ID, err)
	}
	return conversation, nil
}

func generateSessionID() (string, error) {
	data := make([]byte, 16)
	_, err := rand.Read(data)
	if err != nil {
		return "", fmt.Errorf("生成会话 id 失败：%w", err)
	}
	return hex.EncodeToString(data), nil
}
