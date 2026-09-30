package appserver

import (
	"context"
	"fmt"

	"harness/internal/commands"
)

const (
	commandListMethod   = "command/list"
	commandCallMethod   = "command/call"
	commandReadMethod   = "command/settings/read"
	commandSaveMethod   = "command/settings/save"
	commandDeleteMethod = "command/settings/delete"
)

// 数据。按当前工作区列出可用命令。
type CommandListParams struct {
	Workspace string `json:"workspace,omitempty"`
}

// 数据。对 Client 可见的一条平台命令。
type CommandView struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Name         string `json:"name" jsonschema:"minLength=1"`
	Description  string `json:"description"`
	Scope        string `json:"scope"`
	Source       string `json:"source"`
	ArgumentHint string `json:"argumentHint,omitempty"`
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

// 数据。读取一个用户或工作区命令文件。
type CommandSettingsParams struct {
	Scope     string `json:"scope" jsonschema:"enum=user,enum=workspace"`
	Workspace string `json:"workspace,omitempty"`
}

// 数据。保存时携带刚读到的文件版本，防止覆盖外部修改。
type CommandSaveParams struct {
	Scope     string          `json:"scope" jsonschema:"enum=user,enum=workspace"`
	Workspace string          `json:"workspace,omitempty"`
	Hash      string          `json:"hash"`
	Create    bool            `json:"create"`
	Command   commands.Prompt `json:"command"`
}

// 数据。删除一条命令。
type CommandDeleteParams struct {
	Scope     string `json:"scope" jsonschema:"enum=user,enum=workspace"`
	Workspace string `json:"workspace,omitempty"`
	Hash      string `json:"hash"`
	Name      string `json:"name" jsonschema:"minLength=1"`
}

// BindCommands 显式接入公共命令目录；具体产品命令仍由 Product 接受。
func (s *Server) BindCommands(service commands.Commands) error {
	if service == nil || s.commands != nil {
		return fmt.Errorf("appserver: nil or already bound commands")
	}
	s.commands = service
	if err := Register(s, commandListMethod, s.handleCommandList); err != nil {
		return err
	}
	if err := registerSession(s, commandCallMethod, func(input CommandCallParams) string { return input.SessionID }, s.handleCommandCall); err != nil {
		return err
	}
	if service.Prompts() == nil {
		return nil
	}
	if err := Register(s, commandReadMethod, s.handleCommandRead); err != nil {
		return err
	}
	if err := Register(s, commandSaveMethod, s.handleCommandSave); err != nil {
		return err
	}
	return Register(s, commandDeleteMethod, s.handleCommandDelete)
}

func (s *Server) handleCommandList(_ context.Context, input CommandListParams) (CommandListResult, error) {
	definitions := s.commands.List()
	if prompts := s.commands.Prompts(); prompts != nil {
		items, err := prompts.List(input.Workspace)
		if err != nil {
			return CommandListResult{}, methodError(err)
		}
		for _, item := range items {
			definitions = append(definitions, commands.Definition{ID: item.ID, Kind: "prompt", Name: item.Name,
				Description: item.Description, ArgumentHint: item.ArgumentHint, Scope: item.Scope, Source: item.Source})
		}
	}
	result := CommandListResult{Commands: make([]CommandView, 0, len(definitions))}
	for _, definition := range definitions {
		result.Commands = append(result.Commands, CommandView{
			ID: definition.ID, Type: definition.Kind,
			Name:         definition.Name,
			Description:  definition.Description,
			ArgumentHint: definition.ArgumentHint,
			Scope:        definition.Scope, Source: definition.Source,
		})
	}
	return result, nil
}

func (s *Server) handleCommandRead(_ context.Context, input CommandSettingsParams) (commands.PromptFile, error) {
	view, err := s.commands.Prompts().Read(input.Scope, input.Workspace)
	return view, methodError(err)
}

func (s *Server) handleCommandSave(_ context.Context, input CommandSaveParams) (commands.PromptFile, error) {
	view, err := s.commands.Prompts().Save(input.Scope, input.Workspace, input.Hash, input.Command, input.Create)
	return view, methodError(err)
}

func (s *Server) handleCommandDelete(_ context.Context, input CommandDeleteParams) (commands.PromptFile, error) {
	view, err := s.commands.Prompts().Delete(input.Scope, input.Workspace, input.Hash, input.Name)
	return view, methodError(err)
}

func (s *Server) handleCommandCall(ctx context.Context, input CommandCallParams) (CommandCallResult, error) {
	err := s.conversations.CallCommand(ctx, input.Name, input.SessionID)
	return CommandCallResult{}, methodError(err)
}
