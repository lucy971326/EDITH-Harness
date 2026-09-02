package chat

import (
	"testing"

	"harness/kernel/runner"
)

func TestEventHubFiltersSessionAndDropsSlowClient(t *testing.T) {
	hub := newEventHub()
	first, stopFirst := hub.subscribe("first")
	defer stopFirst()
	second, stopSecond := hub.subscribe("second")
	defer stopSecond()
	hub.publish(runner.RunEvent{SessionID: "first", Kind: runner.RunStarted})
	select {
	case event := <-first:
		if event.SessionID != "first" {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("first session did not receive event")
	}
	select {
	case event := <-second:
		t.Fatalf("second session received %#v", event)
	default:
	}

	slow, stopSlow := hub.subscribe("slow")
	defer stopSlow()
	for range cap(slow) + 1 {
		hub.publish(runner.RunEvent{SessionID: "slow", Kind: runner.TextDelta})
	}
	for range cap(slow) {
		<-slow
	}
	if _, ok := <-slow; ok {
		t.Fatal("slow client was not disconnected")
	}
}
