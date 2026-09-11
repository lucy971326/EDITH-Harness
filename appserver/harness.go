package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/events"
	"harness/products/harness"
)

const (
	createMethod    = "harness/session/create"
	listMethod      = "harness/session/list"
	getMethod       = "harness/session/get"
	sendMethod      = "harness/session/send"
	snapshotMethod  = "harness/session/snapshot"
	subscribeMethod = "harness/session/subscribe"
	stopMethod      = "harness/session/stop"
)

// BindHarness 在监听前接入产品与事件来源，登记 Harness 的对外方法。
func (s *Server) BindHarness(product *harness.Product, registry *events.Registry) error {
	if product == nil || registry == nil {
		return fmt.Errorf("appserver: nil harness product or events")
	}
	if s.harnessProduct != nil {
		return fmt.Errorf("appserver: harness already bound")
	}
	s.harnessProduct = product
	s.events = registry
	err := s.registerWorkspaceSelect()
	if err != nil {
		return err
	}
	err = Register(s, createMethod, s.handleCreate)
	if err != nil {
		return err
	}
	err = Register(s, listMethod, s.handleList)
	if err != nil {
		return err
	}
	err = Register(s, getMethod, s.handleGet)
	if err != nil {
		return err
	}
	err = Register(s, sendMethod, s.handleSend)
	if err != nil {
		return err
	}

	err = Register(s, snapshotMethod, s.handleSnapshot)
	if err != nil {
		return err
	}
	err = Register(s, subscribeMethod, s.handleSubscribe)
	if err != nil {
		return err
	}
	return Register(s, stopMethod, s.handleStop)
}

func (s *Server) handleCreate(_ context.Context, input CreateParams) (SessionResult, error) {
	info, err := s.harnessProduct.Create(input.Workspace)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleList(_ context.Context, _ ListParams) (ListResult, error) {
	infos, err := s.harnessProduct.List()
	if err != nil {
		return ListResult{}, methodError(err)
	}
	result := ListResult{Sessions: make([]SessionView, 0, len(infos))}
	for _, info := range infos {
		result.Sessions = append(result.Sessions, sessionView(info))
	}
	return result, nil
}

func (s *Server) handleGet(_ context.Context, input SessionIDParams) (SessionResult, error) {
	info, err := s.harnessProduct.Session(input.SessionID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func sessionView(info harness.SessionInfo) SessionView {
	return SessionView{
		SessionID: info.Meta.ID,
		Title:     info.Meta.Title,
		CreatedAt: info.Meta.CreatedAt,
		Settings:  info.Settings,
	}
}

func methodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, harness.ErrInvalidMessage) {
		return &Error{Code: CodeInvalidParams, Message: "text is empty", Cause: err}
	}
	if errors.Is(err, harness.ErrWorkspace) {
		return &Error{Code: CodeInvalidParams, Message: "workspace is not available", Cause: err}
	}
	if errors.Is(err, harness.ErrInvalidRunSettings) {
		return &Error{Code: CodeInvalidParams, Message: "model, reasoning effort or agent is unavailable", Cause: err}
	}
	// 底层文件缺失不是目标会话不存在，只有产品的明确判断才能映射为未找到。
	if errors.Is(err, harness.ErrSessionNotFound) {
		return &Error{Code: CodeNotFound, Message: "session not found", Cause: err}
	}
	return err
}
