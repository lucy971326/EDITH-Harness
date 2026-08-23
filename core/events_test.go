package core

import (
	"runtime"
	"strings"
	"testing"
)

func TestBroadcastDeliversInSubscribeOrder(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	var order []string
	app.Subscribe("tick", func(payload any) { order = append(order, "第一个") })
	app.Subscribe("tick", func(payload any) { order = append(order, "第二个") })
	app.Subscribe("tick", func(payload any) { order = append(order, "第三个") })

	app.Broadcast("tick", nil)

	if len(order) != 3 {
		t.Fatalf("应通知到全部监听器：got %v", order)
	}
	if order[0] != "第一个" || order[1] != "第二个" || order[2] != "第三个" {
		t.Fatalf("应按注册顺序通知：got %v", order)
	}
}

func TestBroadcastIsSynchronousWithoutNewGoroutines(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	done := make(chan string, 1)
	app.Subscribe("tick", func(payload any) {
		done <- payload.(string)
	})

	before := runtime.NumGoroutine()
	app.Broadcast("tick", "你好")
	after := runtime.NumGoroutine()

	// Broadcast 返回即全部执行完：结果必须已经就位，不需要任何等待。
	select {
	case got := <-done:
		if got != "你好" {
			t.Fatalf("负载应原样送达：got %q", got)
		}
	default:
		t.Fatal("Broadcast 返回后监听器必须已执行完，它必须是同步的")
	}
	if before != after {
		t.Fatalf("Broadcast 不该新增 goroutine：before %d, after %d", before, after)
	}
}

func TestBroadcastIsolatesPanickingListener(t *testing.T) {
	app := New()
	warnings := silentWarn(app)
	t.Cleanup(app.Close)

	var order []string
	app.Subscribe("tick", func(payload any) { panic("第一个炸了") })
	app.Subscribe("tick", func(payload any) { order = append(order, "第二个") })

	app.Broadcast("tick", nil)

	if len(order) != 1 {
		t.Fatalf("崩溃的监听器不该影响后续监听器：got %v", order)
	}
	if len(*warnings) != 1 || !strings.Contains((*warnings)[0], "tick") {
		t.Fatalf("崩溃应隔离为一条点名事件的告警：%v", *warnings)
	}
}

func TestBroadcastOnlyNotifiesMatchingEventName(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	var hits int
	app.Subscribe("chat/chunk", func(payload any) { hits++ })
	app.Subscribe("tool/call", func(payload any) { hits++ })

	app.Broadcast("chat/chunk", nil)

	if hits != 1 {
		t.Fatalf("只有同名事件的监听器该被通知：got %d 次", hits)
	}
}

func TestBroadcastOnEventWithNoListenersIsNoop(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	app.Broadcast("没人听的事件", nil)
}
