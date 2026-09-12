package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/events"
	"harness/kernel/session/settings"
	"harness/products/harness"
)

const (
	createMethod         = "harness/session/create"
	listMethod           = "harness/session/list"
	getMethod            = "harness/session/get"
	updateSettingsMethod = "harness/session/settings/update"
	forkMethod           = "harness/session/fork"
	sendMethod           = "harness/session/send"
	snapshotMethod       = "harness/session/snapshot"
	subscribeMethod      = "harness/session/subscribe"
	stopMethod           = "harness/session/stop"
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
	err = Register(s, updateSettingsMethod, s.handleUpdateSettings)
	if err != nil {
		return err
	}
	err = Register(s, forkMethod, s.handleFork)
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

func (s *Server) handleUpdateSettings(ctx context.Context, input UpdateSettingsParams) (SessionResult, error) {
	info, err := s.harnessProduct.UpdateSettings(ctx, input.SessionID, settings.SessionSettings{
		AgentID:         input.AgentID,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
	})
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleFork(_ context.Context, input ForkParams) (SessionResult, error) {
	destinationID, err := s.harnessProduct.Fork(harness.ForkInput{
		SessionID:       input.SessionID,
		RunID:           input.RunID,
		BoundaryEntryID: input.BoundaryEntryID,
	})
	if err != nil {
		return SessionResult{}, methodError(err)
	}
	info, err := s.harnessProduct.Session(destinationID)
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
	if errors.Is(err, harness.ErrRunChanged) {
		return &Error{Code: CodeConflict, Message: "expected run has ended or changed", Cause: err}
	}
	if errors.Is(err, harness.ErrInvalidMessage) {
		return &Error{Code: CodeInvalidParams, Message: "message is empty", Cause: err}
	}
	if errors.Is(err, harness.ErrWorkspace) {
		return &Error{Code: CodeInvalidParams, Message: "workspace is not available", Cause: err}
	}
	if errors.Is(err, harness.ErrInvalidRunSettings) {
		return &Error{Code: CodeInvalidParams, Message: "model, reasoning effort or agent is unavailable", Cause: err}
	}
	if errors.Is(err, harness.ErrRunActive) {
		return &Error{Code: CodeConflict, Message: "session has an active run", Cause: err}
	}
	if errors.Is(err, harness.ErrInvalidCommand) {
		return &Error{Code: CodeInvalidParams, Message: "command is unavailable", Cause: err}
	}
	if errors.Is(err, harness.ErrCommandRejected) {
		return &Error{Code: CodeConflict, Message: "command cannot run in the current session", Cause: err}
	}
	// 底层文件缺失不是目标会话不存在，只有产品的明确判断才能映射为未找到。
	if errors.Is(err, harness.ErrSessionNotFound) {
		return &Error{Code: CodeNotFound, Message: "session not found", Cause: err}
	}
	return err
}
