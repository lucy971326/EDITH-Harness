package chat

import (
	"net/http/httptest"
	"testing"

	"harness/kernel/runner"
)

func TestEventHubFiltersSessionAndDropsSlowClient(t *testing.T) {
	hub := newEventHub()
	first, stopFirst := hub.subscribe("first")
	defer stopFirst()
	second, stopSecond := hub.subscribe("second")
	defer stopSecond()
	hub.publishRun(runner.RunEvent{SessionID: "first", Kind: runner.RunStarted})
	select {
	case event := <-first:
		if event.run == nil || event.run.SessionID != "first" {
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
		hub.publishRun(runner.RunEvent{SessionID: "slow", Kind: runner.TextDelta})
	}
	for range cap(slow) {
		<-slow
	}
	if _, ok := <-slow; ok {
		t.Fatal("slow client was not disconnected")
	}
}

func TestWriteSSEPreservesMultilineData(t *testing.T) {
	response := httptest.NewRecorder()
	if err := writeSSE(response, "dock-demo", "first\nsecond"); err != nil {
		t.Fatal(err)
	}
	if got, want := response.Body.String(), "event: dock-demo\ndata: first\ndata: second\n\n"; got != want {
		t.Fatalf("SSE = %q, want %q", got, want)
	}
}

func TestEventHubSendsDockChangedOnlyToItsSession(t *testing.T) {
	hub := newEventHub()
	first, stopFirst := hub.subscribe("first")
	defer stopFirst()
	second, stopSecond := hub.subscribe("second")
	defer stopSecond()
	hub.publishDock(DockChanged{SessionID: "first", DockID: "demo"})
	select {
	case event := <-first:
		if event.dock == nil || event.dock.DockID != "demo" {
			t.Fatalf("event = %#v", event)
		}
	default:
		t.Fatal("first session did not receive dock event")
	}
	select {
	case event := <-second:
		t.Fatalf("second session received %#v", event)
	default:
	}
}
