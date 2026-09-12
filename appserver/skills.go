package appserver

import (
	"context"
	"fmt"

	"harness/kernel/skills"
)

const skillListMethod = "skill/list"

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

// BindSkills 显式接入公共 Skill 服务，不经 Product 转发。
func (s *Server) BindSkills(service skills.Skills) error {
	if service == nil || s.skills != nil {
		return fmt.Errorf("appserver: nil or already bound skills")
	}
	s.skills = service
	return Register(s, skillListMethod, s.handleSkillList)
}

func (s *Server) handleSkillList(_ context.Context, input SkillListParams) (SkillListResult, error) {
	info, err := s.harnessProduct.Session(input.SessionID)
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
