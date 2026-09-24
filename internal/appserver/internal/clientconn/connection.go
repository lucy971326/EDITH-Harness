// Package clientconn 管理单个 JSON-RPC Client 的初始化、通知与断线清理。
package clientconn

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	internalrpc "harness/internal/appserver/internal/rpc"

	"github.com/sourcegraph/jsonrpc2"
)

type contextKey struct{}

// 数据。InitializeParams 是本机连接初始化参数。
type InitializeParams struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// 数据。InitializeResult 确认双方使用的协议版本。
type InitializeResult struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// 契约。Caller 校验请求并立即保留业务顺序，返回等待结果的调用。
type Caller func(context.Context, string, json.RawMessage) internalrpc.PreparedCall

// 活对象。Connection 保存一个 Client 的协议状态和瞬时资源。
type Connection struct {
	id     string
	caller Caller
	rpc    *jsonrpc2.Conn
	ctx    context.Context
	cancel context.CancelFunc

	initialized     atomic.Bool
	initializeTimer *time.Timer

	notifications     chan notification
	notificationsDone chan struct{}

	requests      connectionGate
	subscriptions subscriptionSet
}

// New 创建一个连接；调用 Run 后开始通知发送和断线收尾。
func New(parent context.Context, stream jsonrpc2.ObjectStream, caller Caller) *Connection {
	ctx, cancel := context.WithCancel(parent)
	connection := &Connection{
		id:                rand.Text(),
		caller:            caller,
		ctx:               ctx,
		cancel:            cancel,
		notifications:     make(chan notification, notificationQueueLimit),
		notificationsDone: make(chan struct{}),
		subscriptions:     newSubscriptionSet(),
	}
	connection.initializeTimer = time.AfterFunc(10*time.Second, cancel)
	connection.rpc = jsonrpc2.NewConn(ctx, stream, connection, jsonrpc2.SetLogger(discardLogger{}))
	return connection
}

// Run 维持连接直到断开，并等待已开始的请求安全退出。
func (c *Connection) Run() {
	go c.sendNotifications()
	<-c.rpc.DisconnectNotify()

	c.requests.close()
	c.cancel()
	c.initializeTimer.Stop()
	c.requests.wait()
	<-c.notificationsDone
	c.subscriptions.closeAll()
}

func (c *Connection) disconnect() {
	c.cancel()
}

// Handle 继续接收协议消息；业务顺序只由 RPC 方法登记决定。
func (c *Connection) Handle(ctx context.Context, rpcConnection *jsonrpc2.Conn, request *jsonrpc2.Request) {
	if !c.requests.begin() {
		return
	}
	requestContext := &RequestContext{connection: c}
	ctx = context.WithValue(ctx, contextKey{}, requestContext)
	call := c.prepare(ctx, request)
	go func() {
		defer c.requests.end()
		c.handleRequest(ctx, rpcConnection, request, requestContext, call)
	}()
}

type preparedResponse func() (any, error)

func readyResponse(result any, err error) preparedResponse {
	return func() (any, error) {
		return result, err
	}
}

func (c *Connection) handleRequest(ctx context.Context, rpcConnection *jsonrpc2.Conn, request *jsonrpc2.Request, requestContext *RequestContext, call preparedResponse) {
	result, callErr := call()

	if request.Notif {
		requestContext.closeSubscriptions()
		return
	}

	var writeErr error
	if callErr == nil {
		writeErr = rpcConnection.Reply(ctx, request.ID, result)
	} else {
		writeErr = rpcConnection.ReplyWithError(ctx, request.ID, rpcError(callErr))
	}
	if writeErr != nil {
		requestContext.closeSubscriptions()
		c.disconnect()
		return
	}
	if callErr != nil {
		requestContext.closeSubscriptions()
		return
	}
	// 只激活本请求创建的订阅，其他并发请求不能让通知越过它们自己的响应。
	requestContext.activateSubscriptions()
}

func (c *Connection) prepare(ctx context.Context, request *jsonrpc2.Request) preparedResponse {
	params := json.RawMessage(`{}`)
	if request.Params != nil {
		params = *request.Params
	}

	if request.Method == "initialize" {
		if request.Notif {
			return readyResponse(nil, &jsonrpc2.Error{Code: -32001, Message: "Initialization rejected"})
		}
		var input InitializeParams
		err := decodeParams(params, &input)
		if err != nil || input.ProtocolVersion != 1 {
			return readyResponse(nil, &jsonrpc2.Error{Code: -32001, Message: "Initialization rejected"})
		}
		if !c.initialized.CompareAndSwap(false, true) {
			return readyResponse(nil, &jsonrpc2.Error{Code: -32002, Message: "Connection already initialized"})
		}
		c.initializeTimer.Stop()
		return readyResponse(InitializeResult{ProtocolVersion: 1}, nil)
	}

	if !c.initialized.Load() {
		return readyResponse(nil, &jsonrpc2.Error{Code: -32001, Message: "Initialize first"})
	}
	if request.Method == "server/unsubscribe" {
		var input struct {
			SubscriptionID string `json:"subscriptionID"`
		}
		err := decodeParams(params, &input)
		if err != nil || input.SubscriptionID == "" {
			return readyResponse(nil, &internalrpc.Error{Code: internalrpc.CodeInvalidParams, Message: "invalid subscription"})
		}
		return func() (any, error) {
			c.subscriptions.close(input.SubscriptionID)
			return struct{}{}, nil
		}
	}

	call := c.caller(ctx, request.Method, params)
	return func() (any, error) {
		return call()
	}
}

func (c *Connection) sendNotifications() {
	defer close(c.notificationsDone)
	for {
		select {
		case <-c.ctx.Done():
			return
		case item := <-c.notifications:
			if item.sent != nil {
				close(item.sent)
				continue
			}
			err := c.rpc.Notify(c.ctx, item.method, item.params)
			if err != nil {
				c.disconnect()
				return
			}
		}
	}
}

func (c *Connection) enqueueNotification(item notification) bool {
	select {
	case <-c.ctx.Done():
		return false
	case c.notifications <- item:
		return true
	default:
		// 慢 Client 直接断线，不能阻塞 Runner 或静默丢失完成事件。
		c.disconnect()
		return false
	}
}

// FromContext 只供订阅方法取得当前请求的连接能力。
func FromContext(ctx context.Context) (*RequestContext, error) {
	request, ok := ctx.Value(contextKey{}).(*RequestContext)
	if !ok {
		return nil, &internalrpc.Error{Code: internalrpc.CodeConflict, Message: "connection required"}
	}
	return request, nil
}

func decodeParams(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func rpcError(err error) *jsonrpc2.Error {
	var protocol *jsonrpc2.Error
	if errors.As(err, &protocol) {
		return protocol
	}

	var public *internalrpc.Error
	if !errors.As(err, &public) {
		return &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: "Internal error"}
	}

	code := int64(jsonrpc2.CodeInternalError)
	switch public.Code {
	case internalrpc.CodeUnknownMethod:
		code = jsonrpc2.CodeMethodNotFound
	case internalrpc.CodeInvalidParams:
		code = jsonrpc2.CodeInvalidParams
	case internalrpc.CodeNotFound:
		code = -32004
	case internalrpc.CodeConflict:
		code = -32009
	}
	message := public.Message
	if code == jsonrpc2.CodeInternalError {
		message = "Internal error"
	}
	result := &jsonrpc2.Error{Code: code, Message: message}
	result.SetError(public.Code)
	return result
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}
