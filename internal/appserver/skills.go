package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/internal/skills"
)

const (
	skillListMethod             = "skill/list"
	skillSettingsReadMethod     = "skill/settings/read"
	skillSettingsDocumentMethod = "skill/settings/document"
	skillSettingsSaveMethod     = "skill/settings/save"
	skillSettingsToggleMethod   = "skill/settings/toggle"
	skillSettingsDeleteMethod   = "skill/settings/delete"
)

// 数据。Skill 按当前会话的工作区发现。
type SkillListParams struct {
	SessionID string `json:"sessionID" jsonschema:"minLength=1"`
}

// 数据。对 Client 可见的 Skill 摘要；不暴露本机文件路径。
type SkillView struct {
	Name        string `json:"name" jsonschema:"minLength=1"`
	Description string `json:"description"`
	Scope       string `json:"scope" jsonschema:"enum=system,enum=user,enum=workspace"`
}

// 数据。当前会话可用的 Skill 候选。
type SkillListResult struct {
	Skills []SkillView `json:"skills"`
}

// 数据。设置页按当前工作区读取所有来源。
type SkillSettingsReadParams struct {
	Workspace string `json:"workspace"`
}

// 数据。设置页完整清单。
type SkillSettingsReadResult struct {
	Items []skills.SettingsItem `json:"items"`
}

// 数据。根据固定来源和名称读取正文。
type SkillDocumentParams struct {
	Workspace string `json:"workspace"`
	Source    string `json:"source" jsonschema:"minLength=1"`
	Name      string `json:"name" jsonschema:"minLength=1"`
}

// 数据。只保存个人 Skill 的完整 SKILL.md。
type SkillSaveParams struct {
	Name    string `json:"name" jsonschema:"minLength=1"`
	Content string `json:"content"`
	Version string `json:"version"`
	Create  bool   `json:"create"`
}

// 数据。个人 Skill 开关。
type SkillToggleParams struct {
	Name    string `json:"name" jsonschema:"minLength=1"`
	Enabled bool   `json:"enabled"`
}

// 数据。按文件版本删除个人 Skill 目录。
type SkillDeleteParams struct {
	Name    string `json:"name" jsonschema:"minLength=1"`
	Version string `json:"version"`
}

// 数据。写操作成功后的空结果。
type SkillMutationResult struct{}

// BindSkills 显式接入公共 Skill 服务，不经 Product 转发。
func (s *Server) BindSkills(service skills.Skills, settings skills.Settings) error {
	if service == nil || settings == nil || s.skills != nil {
		return fmt.Errorf("appserver: nil or already bound skills")
	}
	s.skills = service
	s.skillSettings = settings
	if err := Register(s, skillListMethod, s.handleSkillList); err != nil {
		return err
	}
	if err := Register(s, skillSettingsReadMethod, s.handleSkillSettingsRead); err != nil {
		return err
	}
	if err := Register(s, skillSettingsDocumentMethod, s.handleSkillDocument); err != nil {
		return err
	}
	if err := Register(s, skillSettingsSaveMethod, s.handleSkillSave); err != nil {
		return err
	}
	if err := Register(s, skillSettingsToggleMethod, s.handleSkillToggle); err != nil {
		return err
	}
	return Register(s, skillSettingsDeleteMethod, s.handleSkillDelete)
}

func (s *Server) handleSkillList(_ context.Context, input SkillListParams) (SkillListResult, error) {
	info, err := s.conversations.Session(input.SessionID)
	if err != nil {
		return SkillListResult{}, methodError(err)
	}
	available, err := s.skills.List(info.Settings.Workspace)
	if err != nil {
		return SkillListResult{}, err
	}
	result := SkillListResult{Skills: make([]SkillView, 0, len(available))}
	for _, skill := range available {
		result.Skills = append(result.Skills, SkillView{
			Name:        skill.Name,
			Description: skill.Description,
			Scope:       string(skill.Scope),
		})
	}
	return result, nil
}

func (s *Server) handleSkillSettingsRead(_ context.Context, input SkillSettingsReadParams) (SkillSettingsReadResult, error) {
	items, err := s.skillSettings.Catalog(input.Workspace)
	if err != nil {
		return SkillSettingsReadResult{}, skillSettingsError(err)
	}
	return SkillSettingsReadResult{Items: items}, nil
}

func (s *Server) handleSkillDocument(_ context.Context, input SkillDocumentParams) (skills.Document, error) {
	document, err := s.skillSettings.ReadDocument(input.Workspace, input.Source, input.Name)
	return document, skillSettingsError(err)
}

func (s *Server) handleSkillSave(_ context.Context, input SkillSaveParams) (skills.Document, error) {
	document, err := s.skillSettings.Save(input.Name, input.Content, input.Version, input.Create)
	return document, skillSettingsError(err)
}

func (s *Server) handleSkillToggle(_ context.Context, input SkillToggleParams) (SkillMutationResult, error) {
	err := s.skillSettings.SetEnabled(input.Name, input.Enabled)
	return SkillMutationResult{}, skillSettingsError(err)
}

func (s *Server) handleSkillDelete(_ context.Context, input SkillDeleteParams) (SkillMutationResult, error) {
	err := s.skillSettings.Delete(input.Name, input.Version)
	return SkillMutationResult{}, skillSettingsError(err)
}

func skillSettingsError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, skills.ErrInvalid) {
		return &Error{Code: CodeInvalidParams, Message: err.Error(), Cause: err}
	}
	if errors.Is(err, skills.ErrConflict) {
		return &Error{Code: CodeConflict, Message: "Skill 文件已更改，请重新读取", Cause: err}
	}
	if errors.Is(err, skills.ErrNotFound) {
		return &Error{Code: CodeNotFound, Message: "Skill 不存在", Cause: err}
	}
	return err
}
