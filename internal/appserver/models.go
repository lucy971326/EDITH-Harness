package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/internal/llm"
)

// 数据。模型目录不接受过滤参数。
type ModelListParams struct{}

// 数据。只公开可选模型，不包含 Provider 配置和密钥。
type ModelListResult struct {
	Models []llm.ModelChoice `json:"models"`
}

// BindModels 显式接入公共模型服务，不经产品转发，也不启动模型调用。
func (s *Server) BindModels(client *llm.Client) error {
	if client == nil || s.models != nil {
		return fmt.Errorf("appserver: nil or already bound models")
	}
	s.models = client
	if err := Register(s, "model/list", s.handleModelList); err != nil {
		return err
	}
	if err := Register(s, "model/config/read", s.handleModelConfigRead); err != nil {
		return err
	}
	if err := Register(s, "model/provider/save", s.handleModelProviderSave); err != nil {
		return err
	}
	if err := Register(s, "model/provider/delete", s.handleModelProviderDelete); err != nil {
		return err
	}
	if err := Register(s, "model/definition/save", s.handleModelDefinitionSave); err != nil {
		return err
	}
	return Register(s, "model/definition/delete", s.handleModelDefinitionDelete)
}

func (s *Server) handleModelList(_ context.Context, _ ModelListParams) (ModelListResult, error) {
	return ModelListResult{Models: s.models.Models()}, nil
}

// 数据。删除供应商时同时核对两份配置版本。
type ModelProviderDeleteParams struct {
	ID               string `json:"id"`
	ProviderRevision string `json:"providerRevision"`
	ModelRevision    string `json:"modelRevision"`
}

// 数据。删除模型时核对目录版本。
type ModelDefinitionDeleteParams struct {
	Key      string `json:"key"`
	Revision string `json:"revision"`
}

func (s *Server) handleModelConfigRead(_ context.Context, _ ModelListParams) (llm.SettingsView, error) {
	return s.models.ReadSettings()
}

func (s *Server) handleModelProviderSave(_ context.Context, input llm.SaveProviderInput) (llm.SettingsView, error) {
	view, err := s.models.SaveProvider(input)
	return view, modelSettingsError(err)
}

func (s *Server) handleModelProviderDelete(_ context.Context, input ModelProviderDeleteParams) (llm.SettingsView, error) {
	view, err := s.models.DeleteProvider(input.ID, input.ProviderRevision, input.ModelRevision)
	return view, modelSettingsError(err)
}

func (s *Server) handleModelDefinitionSave(_ context.Context, input llm.SaveModelInput) (llm.SettingsView, error) {
	view, err := s.models.SaveModel(input)
	return view, modelSettingsError(err)
}

func (s *Server) handleModelDefinitionDelete(_ context.Context, input ModelDefinitionDeleteParams) (llm.SettingsView, error) {
	view, err := s.models.DeleteModel(input.Key, input.Revision)
	return view, modelSettingsError(err)
}

func modelSettingsError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, llm.ErrInvalidSettings):
		return &Error{Code: CodeInvalidParams, Message: err.Error(), Cause: err}
	case errors.Is(err, llm.ErrSettingsChanged):
		return &Error{Code: CodeConflict, Message: "配置文件已被外部修改，请重新加载", Cause: err}
	case errors.Is(err, llm.ErrSettingsMissing):
		return &Error{Code: CodeNotFound, Message: "配置项不存在", Cause: err}
	default:
		return err
	}
}
