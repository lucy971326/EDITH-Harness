package appserver

import (
	"context"
	"fmt"

	"harness/kernel/llm"
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
	return Register(s, "model/list", s.handleModelList)
}

func (s *Server) handleModelList(_ context.Context, _ ModelListParams) (ModelListResult, error) {
	return ModelListResult{Models: s.models.Models()}, nil
}
