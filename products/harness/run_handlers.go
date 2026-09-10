package harness

import (
	"context"
	"strings"

	"harness/appserver"
	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/kernel/session"
)

// 运行接口的接线依赖；Product 本身不保存连接或订阅。
type runHandlers struct {
	// 产品业务与订阅所需的事件来源。
	product *Product
	events  *events.Registry
}

func (p *Product) send(ctx context.Context, input SendParams) (SendResult, error) {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	err := ctx.Err()
	if err != nil {
		return SendResult{}, err
	}
	_, err = p.Session(input.SessionID)
	if err != nil {
		return SendResult{}, methodError(err)
	}
	if strings.TrimSpace(input.Text) == "" {
		return SendResult{}, &appserver.Error{Code: appserver.CodeInvalidParams, Message: "text is empty"}
	}
	message := session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: input.Text}}}
	if _, running := p.runner.State(input.SessionID); running {
		err = p.Steer(input.SessionID, message)
		return SendResult{Mode: "steered"}, err
	}
	runInput := RunInput{
		SessionID:       input.SessionID,
		AgentID:         input.AgentID,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		Message:         message,
	}
	// 接受之后由 Runner 的 Stop / Close 管生命周期，不继承连接取消。
	err = p.Start(context.Background(), runInput)
	if err != nil {
		return SendResult{}, methodError(err)
	}
	return SendResult{Mode: "started"}, nil
}

func (h *runHandlers) snapshot(_ context.Context, input SessionIDParams) (Snapshot, error) {
	_, err := h.product.Session(input.SessionID)
	if err != nil {
		return Snapshot{}, methodError(err)
	}
	return h.product.Snapshot(input.SessionID)
}

func (h *runHandlers) stop(_ context.Context, input SessionIDParams) (StopResult, error) {
	_, err := h.product.Session(input.SessionID)
	if err != nil {
		return StopResult{}, methodError(err)
	}
	return StopResult{}, h.product.Stop(input.SessionID)
}

func (h *runHandlers) subscribe(ctx context.Context, input SessionIDParams) (SubscribeResult, error) {
	_, err := h.product.Session(input.SessionID)
	if err != nil {
		return SubscribeResult{}, methodError(err)
	}
	connection, err := appserver.ConnectionFrom(ctx)
	if err != nil {
		return SubscribeResult{}, err
	}
	subscription, err := connection.Subscribe()
	if err != nil {
		return SubscribeResult{}, err
	}
	listener := &runListener{sessionID: input.SessionID, subscription: subscription}
	unlisten, err := events.Subscribe(h.events, listener.receive)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	subscription.SetCleanup(unlisten)
	// 监听已生效，读快照期间的事件先缓冲；响应入队之后连接才发送它们。
	snapshot, err := h.product.Snapshot(input.SessionID)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	return SubscribeResult{SubscriptionID: subscription.ID, Snapshot: snapshot}, nil
}

type runListener struct {
	sessionID    string
	subscription *appserver.Subscription
}

func (l *runListener) receive(_ context.Context, event runner.RunEvent) error {
	if event.SessionID != l.sessionID {
		return nil
	}
	l.subscription.Notify("harness/run/event", event)
	return nil
}
