//go:build windows

package appserver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestWindowsPickerAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := chooseWorkspace(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before opening, got %v", err)
	}
}

// 在交互式 Windows 桌面显式运行；不把交叉编译当作原生弹窗验收。
func TestWindowsPickerNativeCancellation(t *testing.T) {
	if os.Getenv("HARNESS_TEST_NATIVE_PICKER") != "1" {
		t.Skip("requires interactive Windows desktop and HARNESS_TEST_NATIVE_PICKER=1")
	}
	for _, delay := range []time.Duration{time.Millisecond, 300 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), delay)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := chooseWorkspace(ctx)
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected deadline, got %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("native dialog did not close after cancellation")
			}
			count := 0
			dialogTimers.Range(func(_, _ any) bool { count++; return true })
			if count != 0 {
				t.Fatalf("dialog timer state leaked: %d", count)
			}
		})
	}
}
