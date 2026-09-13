package clientconn

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	internalrpc "harness/appserver/internal/rpc"

	"github.com/sourcegraph/jsonrpc2"
)

func TestHandleReservesOrderBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	preparing := make(chan struct{})
	continuePreparation := make(chan struct{})
	connection := &Connection{
		caller: func(context.Context, string, json.RawMessage) internalrpc.PreparedCall {
			close(preparing)
			<-continuePreparation
			return func() (json.RawMessage, error) { return json.RawMessage(`{}`), nil }
		},
		ctx:           ctx,
		cancel:        cancel,
		notifications: make(chan notification, 1),
		subscriptions: newSubscriptionSet(),
	}
	connection.initialized.Store(true)

	handleReturned := make(chan struct{})
	go func() {
		connection.Handle(ctx, nil, &jsonrpc2.Request{Method: "test/write", Notif: true})
		close(handleReturned)
	}()
	<-preparing
	select {
	case <-handleReturned:
		t.Fatal("Handle returned before the request reserved its execution order")
	case <-time.After(10 * time.Millisecond):
	}

	close(continuePreparation)
	<-handleReturned
	connection.requests.wait()
}
