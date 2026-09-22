package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/appserver/internal/clientconn"
	"harness/kernel/approvals"
)

// BindApprovals 接入独立审批服务；连接仅转发快照与回答。
func (s *Server) BindApprovals(service *approvals.Service) error {
	if service == nil || s.approvals != nil {
		return fmt.Errorf("appserver: nil or already bound approvals")
	}
	s.approvals = service
	err := Register(s, "approval/subscribe", s.handleApprovalSubscribe)
	if err != nil {
		return err
	}
	err = Register(s, "approval/respond", s.handleApprovalRespond)
	if err != nil {
		return err
	}
	err = Register(s, "approval/settings/read", s.handleApprovalSettingsRead)
	if err != nil {
		return err
	}
	err = Register(s, "approval/settings/update", s.handleApprovalSettingsUpdate)
	if err != nil {
		return err
	}
	return Register(s, "permissions/modes", s.handlePermissionModes)
}

func (s *Server) handlePermissionModes(_ context.Context, _ PermissionModesParams) (PermissionModesResult, error) {
	return PermissionModesResult{Modes: s.approvals.Modes()}, nil
}

func (s *Server) handleApprovalSettingsRead(_ context.Context, _ ApprovalSettingsParams) (approvals.SettingsView, error) {
	return s.approvals.Settings(), nil
}

func (s *Server) handleApprovalSettingsUpdate(_ context.Context, input approvals.Settings) (approvals.SettingsView, error) {
	view, err := s.approvals.SaveSettings(input)
	if errors.Is(err, approvals.ErrSettings) {
		return view, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}
	return view, err
}

func (s *Server) handleApprovalSubscribe(ctx context.Context, _ ApprovalSubscribeParams) (ApprovalSubscribeResult, error) {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return ApprovalSubscribeResult{}, err
	}
	subscription, err := request.Subscribe()
	if err != nil {
		return ApprovalSubscribeResult{}, err
	}
	snapshot, updates, unsubscribe := s.approvals.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-subscription.Done():
				return
			case snapshot, ok := <-updates:
				if !ok {
					return
				}
				subscription.Notify("approval/changed", snapshot)
			}
		}
	}()
	subscription.SetCleanup(func() { unsubscribe(); <-done })
	return ApprovalSubscribeResult{SubscriptionID: subscription.ID(), Pending: snapshot}, nil
}

func (s *Server) handleApprovalRespond(_ context.Context, input ApprovalRespondParams) (ApprovalRespondResult, error) {
	err := s.approvals.Respond(input.RequestID, input.Decision)
	if errors.Is(err, approvals.ErrExpired) {
		return ApprovalRespondResult{}, &Error{Code: CodeConflict, Message: err.Error()}
	}
	return ApprovalRespondResult{}, err
}
