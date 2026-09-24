package appserver

import (
	"context"
	"encoding/json"
	"harness/internal/approvals"
	"harness/internal/permissions"
	"net/http"
	"strings"
	"testing"
	"time"

	"harness/internal/appserver/internal/clientconn"

	"github.com/coder/websocket"
)

func startTestSocket(t *testing.T, server *Server) (*Server, string) {
	t.Helper()
	url, err := server.Listen("127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, "ws" + strings.TrimPrefix(url, "http") + "/rpc"
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

func dialTestSocket(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.Dial(t.Context(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	return ws
}

func socketRequest(t *testing.T, ws *websocket.Conn, raw string) rpcResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	err := ws.Write(ctx, websocket.MessageText, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var result rpcResponse
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func initializeSocket(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	result := socketRequest(t, ws, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if string(result.Result) != `{"protocolVersion":1}` {
		t.Fatalf("unexpected initialization result: %s", result.Result)
	}
}

func TestWebSocketInitializationAndOrigin(t *testing.T) {
	rpc := newEchoServer(t)
	s, url := startTestSocket(t, rpc)
	ws := dialTestSocket(t, url)
	response := socketRequest(t, ws, `{"jsonrpc":"2.0","id":1,"method":"echo","params":{"value":"private"}}`)
	if response.Error == nil || response.Error.Code != -32001 {
		t.Fatal(response)
	}
	response = socketRequest(t, ws, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":2}}`)
	if response.Error == nil || response.Error.Code != -32001 {
		t.Fatal(response)
	}
	initializeSocket(t, ws)
	response = socketRequest(t, ws, `{"jsonrpc":"2.0","id":"echo-id","method":"echo","params":{"value":"ok"}}`)
	if response.Error != nil || string(response.ID) != `"echo-id"` || string(response.Result) != `{"value":"ok"}` {
		t.Fatal(response)
	}
	response = socketRequest(t, ws, `{"jsonrpc":"2.0","id":3,"method":"server/catalog"}`)
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatal("removed catalog method is still available", response)
	}
	_, res, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}}})
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatal("foreign origin accepted", err)
	}
	_, res, err = websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://127.0.0.1:5173"}}})
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatal("vite origin accepted without proxy rewrite", err)
	}
	_, err = New().Listen("0.0.0.0:0", nil)
	if err == nil {
		t.Fatal("public listener accepted")
	}
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWebAndRPCShareListener(t *testing.T) {
	server := newEchoServer(t)
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("react"))
	})
	url, err := server.Listen("127.0.0.1:0", web)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.Status)
	}

	ws := dialTestSocket(t, "ws"+strings.TrimPrefix(url, "http")+"/rpc")
	initializeSocket(t, ws)
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
	s, url := startTestSocket(t, rpc)
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)
	response := socketRequest(t, ws, `{"jsonrpc":"2.0","id":2,"method":"subscribe"}`)
	if response.Error != nil || string(response.ID) != "2" {
		t.Fatal("notification overtook response", response)
	}
	<-h.subscription
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
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
	_ = ws.CloseNow()
	_ = s.Close()
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
			s, url := startTestSocket(t, rpc)
			ws := dialTestSocket(t, url)
			initializeSocket(t, ws)
			err = ws.Write(t.Context(), websocket.MessageText, []byte(`{"jsonrpc":"2.0","id":2,"method":"wait"}`))
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
				err = ws.CloseNow()
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
			go func() { done <- s.Close() }()
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
	_, url := startTestSocket(t, server)
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
		ws := dialTestSocket(t, url)
		initializeSocket(t, ws)
		response := socketRequest(t, ws, `{"jsonrpc":"2.0","id":2,"method":"approval/subscribe","params":{}}`)
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
		ws.CloseNow()
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
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)
	response := socketRequest(t, ws, `{"jsonrpc":"2.0","id":3,"method":"approval/subscribe","params":{}}`)
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
