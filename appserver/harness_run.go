package appserver

import (
	"context"

	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/products/harness"
)

func (s *Server) handleSend(ctx context.Context, input SendParams) (SendResult, error) {
	mode, err := s.harnessProduct.Send(ctx, harness.RunInput{
		SessionID:       input.SessionID,
		AgentID:         input.AgentID,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		Message:         session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: input.Text}}},
	})
	return SendResult{Mode: mode}, methodError(err)
}

func (s *Server) handleSnapshot(_ context.Context, input SessionIDParams) (harness.Snapshot, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return harness.Snapshot{}, methodError(err)
	}
	return s.harnessProduct.Snapshot(input.SessionID)
}

func (s *Server) handleStop(_ context.Context, input SessionIDParams) (StopResult, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return StopResult{}, methodError(err)
	}
	return StopResult{}, s.harnessProduct.Stop(input.SessionID)
}

func (s *Server) handleSubscribe(ctx context.Context, input SessionIDParams) (SubscribeResult, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return SubscribeResult{}, methodError(err)
	}
	connection, err := ConnectionFrom(ctx)
	if err != nil {
		return SubscribeResult{}, err
	}
	subscription, err := connection.Subscribe()
	if err != nil {
		return SubscribeResult{}, err
	}
	listener := &runListener{sessionID: input.SessionID, subscription: subscription}
	unlisten, err := events.Subscribe(s.events, listener.receive)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	subscription.SetCleanup(unlisten)
	// 监听已生效，读快照期间的事件先缓冲；响应入队之后连接才发送它们。
	snapshot, err := s.harnessProduct.Snapshot(input.SessionID)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	return SubscribeResult{SubscriptionID: subscription.ID, Snapshot: snapshot}, nil
}

type runListener struct {
	sessionID    string
	subscription *Subscription
}

func (l *runListener) receive(_ context.Context, event runner.RunEvent) error {
	if event.SessionID != l.sessionID {
		return nil
	}
	l.subscription.Notify("harness/run/event", event)
	return nil
}
