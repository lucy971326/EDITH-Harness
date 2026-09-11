package appserver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const selectWorkspaceMethod = "workspace/select"

// 数据。目录选择不接受参数。
type SelectWorkspaceParams struct{}

// 数据。取消时 canceled=true 且 workspace 为空字符串，不作为错误。
type SelectWorkspaceResult struct {
	Canceled  bool   `json:"canceled"`
	Workspace string `json:"workspace"`
}

func (s *Server) registerWorkspaceSelect() error {
	return Register(s, selectWorkspaceMethod, s.handleSelectWorkspace)
}

func (s *Server) handleSelectWorkspace(ctx context.Context, _ SelectWorkspaceParams) (SelectWorkspaceResult, error) {
	workspace, err := selectWorkspace(ctx)
	if errors.Is(err, errWorkspaceCanceled) {
		return SelectWorkspaceResult{Canceled: true}, nil
	}
	if err != nil {
		return SelectWorkspaceResult{}, err
	}
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "" || workspace == "." {
		return SelectWorkspaceResult{Canceled: true}, nil
	}
	if !filepath.IsAbs(workspace) {
		return SelectWorkspaceResult{}, fmt.Errorf("workspace is not an absolute path")
	}
	return SelectWorkspaceResult{Canceled: false, Workspace: workspace}, nil
}
