package appserver

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"harness/internal/approvals"
	"harness/internal/appserver/internal/clientconn"
	"harness/internal/permissions"

	"github.com/sourcegraph/jsonrpc2"
)

func TestServeStreamUsesSameProtocol(t *testing.T) {
	server := newEchoServer(t)
	client, remote := net.Pipe()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() {
		server.ServeStream(jsonrpc2.NewPlainObjectStream(remote))
		close(done)
	}()
	stream := jsonrpc2.NewPlainObjectStream(client)
	defer stream.Close()
	for _, request := range []string{
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":"echo","method":"echo","params":{"value":"desktop"}}`,
	} {
		var response rpcResponse
		if err := stream.WriteObject(json.RawMessage(request)); err != nil {
			t.Fatal(err)
		}
		if err := stream.ReadObject(&response); err != nil {
			t.Fatal(err)
		}
		if response.JSONRPC != "2.0" || response.Error != nil {
			t.Fatal(response)
		}
		if string(response.ID) == `"echo"` && string(response.Result) != `{"value":"desktop"}` {
			t.Fatal(response)
		}
	}
	stream.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not close")
	}
}

func connectTestStream(t *testing.T, server *Server) jsonrpc2.ObjectStream {
	t.Helper()
	t.Cleanup(func() { _ = server.Close() })
	client, remote := net.Pipe()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	stream := jsonrpc2.NewPlainObjectStream(client)
	done := make(chan struct{})
	go func() { server.ServeStream(jsonrpc2.NewPlainObjectStream(remote)); close(done) }()
	t.Cleanup(func() {
		stream.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("connection did not close")
		}
	})
	return stream
}

func streamRequest(t *testing.T, stream jsonrpc2.ObjectStream, raw string) rpcResponse {
	t.Helper()
	err := stream.WriteObject(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	var response rpcResponse
	err = stream.ReadObject(&response)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type echoInput struct {
	Value string `json:"value"`
}

func newEchoServer(t *testing.T) *Server {
	t.Helper()
	server := New()
	err := Register(server, "echo", func(_ context.Context, input echoInput) (echoInput, error) {
		return input, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func initializeStream(t *testing.T, stream jsonrpc2.ObjectStream) {
	t.Helper()
	result := streamRequest(t, stream, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if string(result.Result) != `{"protocolVersion":1}` {
		t.Fatalf("unexpected initialization result: %s", result.Result)
	}
}

type subscriptionHandler struct {
	subscription chan *clientconn.Subscription
	cleaned      chan struct{}
}
type subscriptionReply struct {
	ID string `json:"id"`
}

func (h *subscriptionHandler) call(ctx context.Context, _ struct{}) (subscriptionReply, error) {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return subscriptionReply{}, err
	}
	s, err := request.Subscribe()
	if err != nil {
		return subscriptionReply{}, err
	}
	s.SetCleanup(func() { close(h.cleaned) })
	s.Notify("test/event", "during snapshot")
	h.subscription <- s
	return subscriptionReply{ID: s.ID()}, nil
}

func TestSubscriptionResponsePrecedesEventsAndDisconnectCleans(t *testing.T) {
	rpc := New()
	h := &subscriptionHandler{subscription: make(chan *clientconn.Subscription, 1), cleaned: make(chan struct{})}
	err := Register(rpc, "subscribe", h.call)
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.Close()
	stream := connectTestStream(t, rpc)
	initializeStream(t, stream)
	response := streamRequest(t, stream, `{"jsonrpc":"2.0","id":2,"method":"subscribe"}`)
	if response.Error != nil || string(response.ID) != "2" {
		t.Fatal("notification overtook response", response)
	}
	<-h.subscription
	var raw json.RawMessage
	err = stream.ReadObject(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var notification struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(raw, &notification)
	if notification.Method != "test/event" {
		t.Fatalf("bad event: %s", raw)
	}
	_ = stream.Close()
	_ = rpc.Close()
	select {
	case <-h.cleaned:
	case <-time.After(time.Second):
		t.Fatal("subscription cleanup was not called")
	}
}

type cancellingHandler struct {
	entered   chan struct{}
	cancelled chan struct{}
}

func (h *cancellingHandler) call(ctx context.Context, _ struct{}) (struct{}, error) {
	close(h.entered)
	<-ctx.Done()
	close(h.cancelled)
	return struct{}{}, ctx.Err()
}

func TestCloseCancelsConnectionCallsAndWaits(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "server close"
		if disconnect {
			name = "client disconnect"
		}
		t.Run(name, func(t *testing.T) {
			rpc := New()
			h := &cancellingHandler{entered: make(chan struct{}), cancelled: make(chan struct{})}
			err := Register(rpc, "wait", h.call)
			if err != nil {
				t.Fatal(err)
			}
			defer rpc.Close()
			stream := connectTestStream(t, rpc)
			initializeStream(t, stream)
			err = stream.WriteObject(json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"wait"}`))
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-h.entered:
			case <-time.After(time.Second):
				t.Fatal("handler not called")
			}
			// 同一套连接与等待 handler，分别验收服务器关闭和客户端断线。
			if disconnect {
				err = stream.Close()
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-h.cancelled:
				case <-time.After(time.Second):
					t.Fatal("disconnect did not cancel waiting handler")
				}
			}
			done := make(chan error, 1)
			go func() { done <- rpc.Close() }()
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("close blocked")
			}
			select {
			case <-h.cancelled:
			default:
				t.Fatal("close returned before handler exited")
			}
		})
	}
}

// 验证真实协议中的待审批恢复和输出 Schema，避免页面收到不可用的快照。
func TestApprovalReconnect(t *testing.T) {
	service := approvals.New()
	defer service.Close()
	server := New()
	err := server.BindApprovals(service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	_, updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := service.Authorize(ctx, approvals.Identity{SessionID: "s", RunID: "r", ToolCallID: "c"}, permissions.HumanReviewer, permissions.ApprovalRequest{ToolName: "exec_command", Arguments: []byte(`{"cmd":"curl example.com"}`), Requested: permissions.ExtraPermissions{Network: true}})
		finished <- err
	}()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("request missing")
	}
	var id string
	for i := 0; i < 2; i++ {
		stream := connectTestStream(t, server)
		initializeStream(t, stream)
		response := streamRequest(t, stream, `{"jsonrpc":"2.0","id":2,"method":"approval/subscribe","params":{}}`)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var snapshot ApprovalSubscribeResult
		err = json.Unmarshal(response.Result, &snapshot)
		if err != nil || len(snapshot.Pending) != 1 {
			t.Fatalf("snapshot %s: %v", response.Result, err)
		}
		if id != "" && id != snapshot.Pending[0].ID {
			t.Fatal("reconnect replaced approval")
		}
		id = snapshot.Pending[0].ID
		stream.Close()
	}
	_, err = server.Call(t.Context(), "approval/respond", mustJSON(t, ApprovalRespondParams{RequestID: id, Decision: permissions.Decision{Approved: true}}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("answer did not resume tool")
	}
	go func() {
		finished <- service.AuthorizeMCPCall(ctx, approvals.Identity{SessionID: "s", RunID: "r", ToolCallID: "mcp"}, permissions.HumanReviewer,
			approvals.MCPRequest{Kind: "call", Workspace: "/work", Server: "demo", Tool: "ping", Arguments: []byte(`{}`)})
	}()
	deadline := time.After(time.Second)
	var mcpPending []approvals.Pending
	for len(mcpPending) == 0 {
		select {
		case mcpPending = <-updates:
		case <-deadline:
			t.Fatal("MCP approval request missing")
		}
	}
	stream := connectTestStream(t, server)
	initializeStream(t, stream)
	response := streamRequest(t, stream, `{"jsonrpc":"2.0","id":3,"method":"approval/subscribe","params":{}}`)
	if response.Error != nil {
		t.Fatalf("MCP approval snapshot failed output validation: %#v", response.Error)
	}
	var snapshot ApprovalSubscribeResult
	if err := json.Unmarshal(response.Result, &snapshot); err != nil || len(snapshot.Pending) != 1 || snapshot.Pending[0].MCP == nil {
		t.Fatalf("MCP approval snapshot = %s, %v", response.Result, err)
	}
	if err := service.Respond(snapshot.Pending[0].ID, permissions.Decision{Approved: false}); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err == nil {
		t.Fatal("denied MCP call was approved")
	}
}
