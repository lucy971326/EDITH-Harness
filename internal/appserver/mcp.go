package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/internal/tools/mcp"
)

// 数据。读取某工作区的 MCP 配置和已有连接状态。
type MCPReadParams struct {
	Workspace string `json:"workspace"`
}

// 数据。按版本删除一个全局 Server。
type MCPDeleteParams struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

// 数据。显式重试一个 Server 的连接。
type MCPRetryParams struct {
	Name string `json:"name"`
}

// 数据。用户明确触发损坏配置备份与重建。
type MCPResetParams struct {
	Revision string `json:"revision"`
}

// BindMCP 登记设置页的 MCP 配置方法。
func (s *Server) BindMCP(provider *mcp.Provider) error {
	if provider == nil || s.mcp != nil {
		return fmt.Errorf("appserver: nil or already bound MCP provider")
	}
	s.mcp = provider
	if err := Register(s, "mcp/read", s.handleMCPRead); err != nil {
		return err
	}
	if err := Register(s, "mcp/save", s.handleMCPSave); err != nil {
		return err
	}
	if err := Register(s, "mcp/delete", s.handleMCPDelete); err != nil {
		return err
	}
	if err := Register(s, "mcp/retry", s.handleMCPRetry); err != nil {
		return err
	}
	return Register(s, "mcp/resetInvalid", s.handleMCPReset)
}

func (s *Server) handleMCPRead(_ context.Context, input MCPReadParams) (mcp.SettingsView, error) {
	view, err := s.mcp.ReadSettings(input.Workspace)
	return view, mcpSettingsError(err)
}

func (s *Server) handleMCPSave(ctx context.Context, input mcp.SaveInput) (mcp.SettingsView, error) {
	view, err := s.mcp.Save(ctx, input)
	return view, mcpSettingsError(err)
}

func (s *Server) handleMCPDelete(ctx context.Context, input MCPDeleteParams) (mcp.SettingsView, error) {
	view, err := s.mcp.Delete(ctx, input.Name, input.Revision)
	return view, mcpSettingsError(err)
}

func (s *Server) handleMCPRetry(ctx context.Context, input MCPRetryParams) (mcp.SettingsView, error) {
	view, err := s.mcp.Retry(ctx, input.Name)
	return view, mcpSettingsError(err)
}

func (s *Server) handleMCPReset(ctx context.Context, input MCPResetParams) (mcp.SettingsView, error) {
	view, err := s.mcp.ResetInvalid(ctx, input.Revision)
	return view, mcpSettingsError(err)
}

func mcpSettingsError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mcp.ErrChanged):
		return &Error{Code: CodeConflict, Message: "MCP 配置已改变，请重新加载"}
	case errors.Is(err, mcp.ErrMissing):
		return &Error{Code: CodeNotFound, Message: "MCP Server 不存在"}
	case errors.Is(err, mcp.ErrInvalid):
		return &Error{Code: CodeInvalidParams, Message: err.Error()}
	default:
		return &Error{Code: CodeInternal, Message: "MCP 设置操作失败"}
	}
}
