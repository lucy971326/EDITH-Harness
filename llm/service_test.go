package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"harness/chat"
	"harness/core"
)

// fakeAdapter 是测试用的假适配器：可以编排它吐什么字、回什么、什么时候炸。
type fakeAdapter struct {
	name   string
	deltas []string        // 依次吐的字
	reply  Reply           // 吐完后返回的整份回复
	fail   error           // 非空则返回这个错
	sawCtx context.Context // 记下收到的 ctx，供取消测试断言
	delay  time.Duration   // 吐字间隔，给取消测试留时间
}

func (a *fakeAdapter) Name() string {
	return a.name
}

func (a *fakeAdapter) Stream(ctx context.Context, req Request, onDelta func(chat.Delta)) (Reply, error) {
	a.sawCtx = ctx
	for _, text := range a.deltas {
		select {
		case <-ctx.Done():
			return Reply{}, NewError(a.name, ErrCancelled, ctx.Err().Error())
		case <-time.After(a.delay):
		}
		if onDelta != nil {
			onDelta(chat.Delta{Text: text})
		}
	}
	if a.fail != nil {
		return Reply{}, a.fail
	}
	return a.reply, nil
}

func newTestService(t *testing.T) (*Service, *fakeAdapter) {
	t.Helper()

	service := NewService()
	fake := &fakeAdapter{
		name:   "假的",
		deltas: []string{"你", "好"},
		reply:  Reply{Text: "你好", Usage: chat.Usage{InputTokens: 5, OutputTokens: 2}, StopReason: "stop"},
	}
	err := service.Register(fake)
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
	return service, fake
}

func TestStreamDeliversDeltasThenReply(t *testing.T) {
	service, _ := newTestService(t)
	err := service.SetDefault("假的")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	var received []string
	reply, err := service.Stream(context.Background(), Request{Model: "test"},
		func(d chat.Delta) { received = append(received, d.Text) })
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}

	if strings.Join(received, "") != "你好" {
		t.Fatalf("逐字该按顺序全到：got %v", received)
	}
	if reply.Text != "你好" || reply.Usage.OutputTokens != 2 {
		t.Fatalf("回复该是适配器聚合好的整份：got %+v", reply)
	}
}

func TestAdapterErrorBecomesUnifiedError(t *testing.T) {
	service, fake := newTestService(t)
	fake.fail = errors.New("连接被重置")
	err := service.SetDefault("假的")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	_, err = service.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("适配器炸了该报错")
	}

	typed, ok := err.(*Error)
	if !ok {
		t.Fatalf("出口该是统一错误，got %T", err)
	}
	if typed.Provider != "假的" || typed.Kind != ErrUnknown {
		t.Fatalf("该带上哪家和性质：got %+v", typed)
	}
}

func TestAdapterMayReturnKindedErrorAsIs(t *testing.T) {
	service, fake := newTestService(t)
	fake.fail = NewError("假的", ErrRateLimit, "太快了")
	err := service.SetDefault("假的")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	_, err = service.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("该报错")
	}
	typed := err.(*Error)
	if typed.Kind != ErrRateLimit {
		t.Fatalf("适配器已翻译好的性质不该被覆盖：got %+v", typed)
	}
}

func TestRoutesToNamedAdapter(t *testing.T) {
	service := NewService()
	cheap := &fakeAdapter{name: "便宜的", reply: Reply{Text: "便宜家的回复"}}
	dear := &fakeAdapter{name: "贵的", reply: Reply{Text: "贵家的回复"}}
	err := service.Register(cheap)
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
	err = service.Register(dear)
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}

	reply, err := service.CompleteWith(context.Background(), "贵的", Request{})
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	if reply.Text != "贵家的回复" {
		t.Fatalf("该路由到指定那家：got %q", reply.Text)
	}
}

func TestRegisterRejectsDuplicateName(t *testing.T) {
	service := NewService()
	err := service.Register(&fakeAdapter{name: "重的"})
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
	err = service.Register(&fakeAdapter{name: "重的"})
	if err == nil {
		t.Fatal("同名重复插该报错")
	}
}

func TestCompleteWithoutDefaultErrors(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("没设默认插座该报错，不能猜")
	}
}

func TestUnknownProviderErrors(t *testing.T) {
	service, _ := newTestService(t)

	_, err := service.CompleteWith(context.Background(), "不存在的", Request{})
	if err == nil {
		t.Fatal("没登记过的插座该报错")
	}
}

func TestCancellationReachesAdapter(t *testing.T) {
	service, fake := newTestService(t)
	fake.delay = 50 * time.Millisecond
	err := service.SetDefault("假的")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err = service.Stream(ctx, Request{}, nil)
	if err == nil {
		t.Fatal("取消后该收到错误")
	}
	typed, ok := err.(*Error)
	if !ok || typed.Kind != ErrCancelled {
		t.Fatalf("取消该是统一错误的 cancelled 性质：got %v", err)
	}
}

func TestToolCallRoundTripsThroughRequest(t *testing.T) {
	service, fake := newTestService(t)
	fake.reply = Reply{
		Text: "我来查",
		Calls: []chat.ToolCall{{
			ID:       "c1",
			Name:     "bash",
			Argument: json.RawMessage(`{"cmd":"ls"}`),
		}},
		StopReason: "tool_calls",
	}
	err := service.SetDefault("假的")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	reply, err := service.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	if len(reply.Calls) != 1 || reply.Calls[0].Name != "bash" || string(reply.Calls[0].Argument) != `{"cmd":"ls"}` {
		t.Fatalf("工具调用该原样回来：got %+v", reply.Calls)
	}
}

func TestPluginRegistersServiceInApp(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(Plugin{})
	if err != nil {
		t.Fatalf("装插件失败：%v", err)
	}

	service, err := Get(app)
	if err != nil {
		t.Fatalf("取插座排失败：%v", err)
	}
	err = service.Register(&fakeAdapter{name: "假的"})
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
}
