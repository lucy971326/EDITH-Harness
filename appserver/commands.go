package appserver

import (
	"context"
	"fmt"

	"harness/kernel/commands"
)

const (
	commandListMethod = "command/list"
	commandCallMethod = "command/call"
)

// 数据。命令目录不接受过滤参数。
type CommandListParams struct{}

// 数据。对 Client 可见的一条平台命令。
type CommandView struct {
	Name        string `json:"name" jsonschema:"minLength=1"`
	Description string `json:"description"`
}

// 数据。当前已登记的平台命令。
type CommandListResult struct {
	Commands []CommandView `json:"commands"`
}

// 数据。立即执行一条会话命令。
type CommandCallParams struct {
	SessionID string `json:"sessionID" jsonschema:"minLength=1"`
	Name      string `json:"name" jsonschema:"minLength=1"`
}

// 数据。命令已被接受；运行结果继续通过会话订阅取得。
type CommandCallResult struct{}

// BindCommands 显式接入公共命令目录；具体产品命令仍由 Product 接受。
func (s *Server) BindCommands(service commands.Commands) error {
	if service == nil || s.commands != nil {
		return fmt.Errorf("appserver: nil or already bound commands")
	}
	s.commands = service
	if err := Register(s, commandListMethod, s.handleCommandList); err != nil {
		return err
	}
	return Register(s, commandCallMethod, s.handleCommandCall)
}

func (s *Server) handleCommandList(_ context.Context, _ CommandListParams) (CommandListResult, error) {
	definitions := s.commands.List()
	result := CommandListResult{Commands: make([]CommandView, 0, len(definitions))}
	for _, definition := range definitions {
		result.Commands = append(result.Commands, CommandView{
			Name:        definition.Name,
			Description: definition.Description,
		})
	}
	return result, nil
}

func (s *Server) handleCommandCall(ctx context.Context, input CommandCallParams) (CommandCallResult, error) {
	err := s.harnessProduct.CallCommand(ctx, input.Name, input.SessionID)
	return CommandCallResult{}, methodError(err)
}
