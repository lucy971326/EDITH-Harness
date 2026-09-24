package clientconn

import (
	"context"
	"testing"
)

func TestRequestActivatesOnlyItsOwnSubscriptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := &Connection{
		ctx:           ctx,
		cancel:        cancel,
		notifications: make(chan notification, 2),
		subscriptions: newSubscriptionSet(),
	}
	first := &RequestContext{connection: connection}
	second := &RequestContext{connection: connection}
	firstSubscription, err := first.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	secondSubscription, err := second.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	firstSubscription.Notify("event", "first")
	secondSubscription.Notify("event", "second")

	second.activateSubscriptions()
	item := <-connection.notifications
	event := item.params.(subscriptionEvent)
	if event.SubscriptionID != secondSubscription.ID() || len(connection.notifications) != 0 {
		t.Fatal("one request activated another request's subscription")
	}
}

func TestSlowSubscriptionDisconnectsAndLateCleanupRuns(t *testing.T) {
	for _, active := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		connection := &Connection{
			ctx:           ctx,
			cancel:        cancel,
			notifications: make(chan notification, 1),
			subscriptions: newSubscriptionSet(),
		}
		request := &RequestContext{connection: connection}
		subscription, err := request.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		if active {
			request.activateSubscriptions()
		}
		limit := subscriptionPendingLimit + 1
		if active {
			limit = 2
		}
		for range limit {
			subscription.Notify("event", "data")
		}
		if ctx.Err() == nil {
			t.Fatalf("active=%v: slow connection stayed open", active)
		}

		subscription.Close()
		cleaned := false
		subscription.SetCleanup(func() { cleaned = true })
		subscription.Close()
		if !cleaned {
			t.Fatalf("active=%v: late cleanup was lost", active)
		}
	}
}
