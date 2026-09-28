package appserver

import (
	"context"
	"harness/internal/appserver/internal/clientconn"
	"harness/internal/events"
	"harness/internal/runner"
)

// 接入层失效通知，不是第二份会话事实。
type activityChanged struct{}

func (s *Server) invalidateActivity() {
	_ = events.Publish(context.Background(), s.events, activityChanged{})
}

func (s *Server) handleActivityList(_ context.Context, _ ListParams) (ActivityListResult, error) {
	items, err := s.conversations.Activities()
	return ActivityListResult{Sessions: items}, methodError(err)
}

func (s *Server) handleMarkRead(_ context.Context, input MarkReadParams) (StopResult, error) {
	err := s.conversations.MarkRead(input.SessionID, input.RunID)
	if err != nil {
		return StopResult{}, methodError(err)
	}
	s.invalidateActivity()
	return StopResult{}, nil
}

func (s *Server) handleActivitySubscribe(ctx context.Context, _ ListParams) (ActivitySubscribeResult, error) {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return ActivitySubscribeResult{}, err
	}
	subscription, err := request.Subscribe()
	if err != nil {
		return ActivitySubscribeResult{}, err
	}
	stopRuns, err := events.Subscribe(s.events, func(_ context.Context, event runner.RunEvent) error {
		if event.Kind == runner.RunStarted || event.Kind == runner.RunEnded {
			subscription.Notify("harness/session/activity/changed", activityChanged{})
		}
		return nil
	})
	if err != nil {
		subscription.Close()
		return ActivitySubscribeResult{}, err
	}
	stopRead, err := events.Subscribe(s.events, func(_ context.Context, _ activityChanged) error {
		subscription.Notify("harness/session/activity/changed", activityChanged{})
		return nil
	})
	if err != nil {
		stopRuns()
		subscription.Close()
		return ActivitySubscribeResult{}, err
	}
	subscription.SetCleanup(func() { stopRuns(); stopRead() })
	items, err := s.conversations.Activities()
	if err != nil {
		subscription.Close()
		return ActivitySubscribeResult{}, methodError(err)
	}
	return ActivitySubscribeResult{SubscriptionID: subscription.ID(), Sessions: items}, nil
}
