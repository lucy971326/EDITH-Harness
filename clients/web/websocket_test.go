package webclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/internal/appserver"

	"github.com/coder/websocket"
)

func TestListenerFailureAndCloseRace(t *testing.T) {
	rpc := newEchoServer(t)
	web, err := New(rpc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = web.Close() })
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	_, err = web.Listen(occupied.Addr().String())
	if err == nil {
		t.Fatal("occupied address accepted")
	}
	occupied.Close()
	url, err := web.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var clients sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		clients.Add(1)
		go func() {
			defer clients.Done()
			<-start
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/rpc", nil)
			if err == nil {
				ws.CloseNow()
			}
		}()
	}
	close(start)
	done := make(chan error, 1)
	go func() { done <- web.Close() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close raced with handshake and blocked")
	}
	clients.Wait()
	_, err = web.Listen("127.0.0.1:0")
	if err == nil {
		t.Fatal("closed listener restarted")
	}
}
func startTestSocket(t *testing.T, server *appserver.Server) (*Server, string) {
	t.Helper()
	web, err := New(server, nil)
	if err != nil {
		t.Fatal(err)
	}
	url, err := web.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = web.Close() })
	return web, "ws" + strings.TrimPrefix(url, "http") + "/rpc"
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

func newEchoServer(t *testing.T) *appserver.Server {
	t.Helper()
	server := appserver.New()
	err := appserver.Register(server, "echo", func(_ context.Context, input echoInput) (echoInput, error) {
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
	public, err := New(rpc, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = public.Listen("0.0.0.0:0")
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
	transport, err := New(server, web)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })
	url, err := transport.Listen("127.0.0.1:0")
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
			rpc := appserver.New()
			h := &cancellingHandler{entered: make(chan struct{}), cancelled: make(chan struct{})}
			err := appserver.Register(rpc, "wait", h.call)
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
