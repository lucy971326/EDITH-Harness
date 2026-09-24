package events

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type firstEvent struct{ Value int }
type secondEvent struct{ Value int }

func TestRegistryPublishesByTypeInRegistrationOrder(t *testing.T) {
	registry := NewRegistry()
	var got []int

	_, err := Subscribe(registry, func(_ context.Context, event firstEvent) error {
		got = append(got, event.Value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Subscribe(registry, func(_ context.Context, event firstEvent) error {
		got = append(got, event.Value+1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Subscribe(registry, func(context.Context, secondEvent) error {
		t.Fatal("wrong event type was delivered")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = Publish(context.Background(), registry, firstEvent{Value: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestRegistryUnregisterIsIdempotent(t *testing.T) {
	registry := NewRegistry()
	called := 0
	unregister, err := Subscribe(registry, func(context.Context, firstEvent) error {
		called++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	unregister()
	unregister()

	err = Publish(context.Background(), registry, firstEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatalf("called = %d", called)
	}
}

func TestRegistryPublishesToAllAndJoinsErrors(t *testing.T) {
	registry := NewRegistry()
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	called := 0

	_, err := Subscribe(registry, func(context.Context, firstEvent) error {
		called++
		return firstErr
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Subscribe(registry, func(context.Context, firstEvent) error {
		called++
		return secondErr
	})
	if err != nil {
		t.Fatal(err)
	}

	err = Publish(context.Background(), registry, firstEvent{})
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("error = %v", err)
	}
	if called != 2 {
		t.Fatalf("called = %d", called)
	}
}

func TestRegistryConcurrentSubscribePublishAndUnregister(t *testing.T) {
	registry := NewRegistry()
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			unregister, err := Subscribe(registry, func(context.Context, firstEvent) error {
				return nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			if err := Publish(context.Background(), registry, firstEvent{}); err != nil {
				t.Error(err)
			}
			unregister()
		}()
	}
	wait.Wait()
}

func TestRegistryRejectsNilInputs(t *testing.T) {
	_, err := Subscribe[firstEvent](nil, func(context.Context, firstEvent) error { return nil })
	if err == nil {
		t.Fatal("expected nil registry error")
	}
	_, err = Subscribe[firstEvent](NewRegistry(), nil)
	if err == nil {
		t.Fatal("expected nil handler error")
	}
	err = Publish(context.Background(), (*Registry)(nil), firstEvent{})
	if err == nil {
		t.Fatal("expected nil registry error")
	}
}
