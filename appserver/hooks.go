package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/hooks"
	"harness/kernel/machine"
)

// 数据。读取当前工作区的两份 Hook 配置。
type HookReadParams struct {
	Workspace string `json:"workspace"`
}

// BindHooks 接入 Hook 设置；工具执行仍由内核登记处触发。
func (s *Server) BindHooks(service *hooks.Service) error {
	if service == nil || s.hooks != nil {
		return fmt.Errorf("appserver: nil or already bound hooks")
	}
	s.hooks = service
	if err := Register(s, "hooks/read", s.handleHookRead); err != nil {
		return err
	}
	if err := Register(s, "hooks/save", s.handleHookSave); err != nil {
		return err
	}
	return Register(s, "hooks/trust", s.handleHookTrust)
}

func (s *Server) handleHookRead(_ context.Context, input HookReadParams) (hooks.View, error) {
	return s.hooks.View(input.Workspace)
}

func (s *Server) handleHookSave(_ context.Context, input hooks.SaveInput) (hooks.View, error) {
	view, err := s.hooks.Save(input)
	return view, hookMethodError(err)
}

func (s *Server) handleHookTrust(_ context.Context, input hooks.TrustInput) (hooks.View, error) {
	view, err := s.hooks.Trust(input)
	return view, hookMethodError(err)
}

func hookMethodError(err error) error {
	if errors.Is(err, hooks.ErrSettings) {
		return &Error{Code: CodeInvalidParams, Message: err.Error(), Cause: err}
	}
	if errors.Is(err, machine.ErrFileConflict) {
		return &Error{Code: CodeConflict, Message: "Hook 配置已改变，请重新加载", Cause: err}
	}
	return err
}
