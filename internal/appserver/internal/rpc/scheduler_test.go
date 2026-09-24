package rpc

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSchedulerOrdersOneSessionWithoutBlockingAnother(t *testing.T) {
	scheduler := newScheduler()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	otherStarted := make(chan struct{})

	first := scheduler.Schedule(context.Background(), "a", func() error {
		close(firstStarted)
		<-releaseFirst
		return nil
	})
	<-firstStarted
	second := scheduler.Schedule(context.Background(), "a", func() error {
		close(secondStarted)
		return nil
	})
	other := scheduler.Schedule(context.Background(), "b", func() error {
		close(otherStarted)
		return nil
	})

	var wait sync.WaitGroup
	wait.Add(3)
	for _, call := range []func() error{first, second, other} {
		go func() {
			defer wait.Done()
			_ = call()
		}()
	}

	select {
	case <-otherStarted:
	case <-time.After(time.Second):
		t.Fatal("another session was blocked")
	}
	select {
	case <-secondStarted:
		t.Fatal("same-session call overtook the first")
	default:
	}
	close(releaseFirst)
	wait.Wait()
}
