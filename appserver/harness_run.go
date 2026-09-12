package appserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"sync"

	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/products/harness"
)

const maxImageBytes = 2 << 20

var imageMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func (s *Server) handleSend(ctx context.Context, input SendParams) (SendResult, error) {
	blocks := make([]session.Block, 0, len(input.Images)+1)
	for _, image := range input.Images {
		data, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil {
			return SendResult{}, invalidImage("image data is not valid base64", err)
		}
		if len(data) == 0 || len(data) > maxImageBytes {
			return SendResult{}, invalidImage("image must be between 1 byte and 2 MB", nil)
		}
		detected := http.DetectContentType(data)
		if !imageMIMEs[image.MIME] || detected != image.MIME {
			return SendResult{}, invalidImage("image MIME does not match its contents", nil)
		}
		blocks = append(blocks, session.Block{
			Kind:  "image",
			Media: &session.Media{MIME: image.MIME, Data: image.Data},
		})
	}
	if input.Text != "" {
		blocks = append(blocks, session.Block{Kind: "text", Text: input.Text})
	}
	mode, err := s.harnessProduct.Send(ctx, harness.RunInput{
		SessionID:     input.SessionID,
		ExpectedRunID: input.ExpectedRunID,
		Message:       session.UserMessage{Blocks: blocks},
	})
	return SendResult{Mode: mode}, methodError(err)
}

func invalidImage(message string, cause error) error {
	return &Error{Code: CodeInvalidParams, Message: message, Cause: cause}
}

func (s *Server) handleSnapshot(_ context.Context, input SessionIDParams) (harness.Snapshot, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return harness.Snapshot{}, methodError(err)
	}
	return s.harnessProduct.Snapshot(input.SessionID)
}

func (s *Server) handleStop(_ context.Context, input SessionIDParams) (StopResult, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return StopResult{}, methodError(err)
	}
	return StopResult{}, s.harnessProduct.Stop(input.SessionID)
}

func (s *Server) handleSubscribe(ctx context.Context, input SessionIDParams) (SubscribeResult, error) {
	_, err := s.harnessProduct.Session(input.SessionID)
	if err != nil {
		return SubscribeResult{}, methodError(err)
	}
	connection, err := ConnectionFrom(ctx)
	if err != nil {
		return SubscribeResult{}, err
	}
	subscription, err := connection.Subscribe()
	if err != nil {
		return SubscribeResult{}, err
	}
	listener := &runListener{sessionID: input.SessionID, subscription: subscription, pending: make(map[uint64]runner.RunEvent)}
	unlisten, err := events.Subscribe(s.events, listener.receive)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	subscription.SetCleanup(unlisten)
	// 监听已生效，读快照期间的事件先缓冲；响应入队之后连接才发送它们。
	// 快照已含当时的账本、草稿和运行状态；Client 用 updateSeq / Entry.ID 丢掉重叠。
	snapshot, err := s.harnessProduct.Snapshot(input.SessionID)
	if err != nil {
		subscription.Close()
		return SubscribeResult{}, err
	}
	listener.start(snapshot)
	return SubscribeResult{SubscriptionID: subscription.ID, Snapshot: snapshot}, nil
}

type runListener struct {
	sessionID    string
	subscription *Subscription

	// 内核同步回调可以重入；这里只整理本订阅的发送顺序，不反压 Runner。
	mu      sync.Mutex
	ready   bool
	epoch   string
	through uint64
	pending map[uint64]runner.RunEvent
}

func (l *runListener) receive(_ context.Context, event runner.RunEvent) error {
	if event.SessionID != l.sessionID {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.subscription.connection.ctx.Err() != nil {
		return nil
	}
	if l.ready && event.SeqEpoch != l.epoch {
		l.subscription.connection.disconnect()
		return nil
	}
	if event.UpdateSeq <= l.through {
		return nil
	}
	l.pending[event.UpdateSeq] = event
	if l.ready {
		l.flush()
	}
	if len(l.pending) > queueLimit {
		l.subscription.connection.disconnect()
	}
	return nil
}

func (l *runListener) start(snapshot harness.Snapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ready = true
	l.epoch = snapshot.SeqEpoch
	l.through = snapshot.UpdateSeq
	for seq, event := range l.pending {
		if event.SeqEpoch != l.epoch {
			l.subscription.connection.disconnect()
			return
		}
		if seq <= l.through {
			delete(l.pending, seq)
		}
	}
	l.flush()
}

// 调用方持 mu；只放行连续更新，缺口和慢连接均受缓冲上限保护。
func (l *runListener) flush() {
	for {
		event, ok := l.pending[l.through+1]
		if !ok {
			return
		}
		delete(l.pending, event.UpdateSeq)
		l.through = event.UpdateSeq
		l.subscription.Notify("harness/run/event", event)
	}
}
