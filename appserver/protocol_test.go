package appserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type echoInput struct {
	Value string `json:"value"`
}
type echoHandler struct{}

func (echoHandler) call(_ context.Context, input echoInput) (echoInput, error) { return input, nil }

func newEchoServer(t *testing.T) *RPCServer {
	t.Helper()
	s := New()
	err := Register(s, Method[echoInput, echoInput]{Name: "echo"}, echoHandler{}.call)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestJSONRPCEnvelope(t *testing.T) {
	s := newEchoServer(t)
	for _, tc := range []struct {
		input string
		code  int
		id    string
	}{
		{`{`, -32700, `null`},
		{`null`, -32600, `null`},
		{`[]`, -32600, `null`},
		{`{"method":"echo","id":1}`, -32600, `null`},
		{`{"jsonrpc":"2.0","method":null,"id":1}`, -32600, `null`},
		{`{"jsonrpc":"2.0","method":"echo","id":false}`, -32600, `null`},
		{`{"jsonrpc":"2.0","method":"missing","id":"x"}`, -32601, `"x"`},
		{`{"jsonrpc":"2.0","method":"echo","params":{},"id":7}`, -32602, `7`},
		{`{"jsonrpc":"2.0","method":"echo","params":[],"id":7}`, -32602, `7`},
		{`{"jsonrpc":"2.0","method":"echo","params":false,"id":7}`, -32602, `7`},
		{`{"jsonrpc":"2.0","method":"echo","params":{"value":"ok"},"id":9007199254740993}`, 0, `9007199254740993`},
		{`{"jsonrpc":"2.0","method":"echo","params":{"value":"ok"},"id":null}`, 0, `null`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			raw := s.Handle(t.Context(), []byte(tc.input))
			var response rpcResponse
			err := json.Unmarshal(raw, &response)
			if err != nil {
				t.Fatal(err)
			}
			if response.JSONRPC != "2.0" || string(response.ID) != tc.id {
				t.Fatalf("bad envelope: %s", raw)
			}
			if tc.code == 0 {
				if response.Error != nil || string(response.Result) != `{"value":"ok"}` {
					t.Fatalf("bad result: %s", raw)
				}
			} else if response.Error == nil || response.Error.Code != tc.code || response.Result != nil {
				t.Fatalf("bad error: %s", raw)
			}
		})
	}
}

func TestJSONRPCNotificationsAndBatch(t *testing.T) {
	s := newEchoServer(t)
	for _, raw := range []string{
		`{"jsonrpc":"2.0","method":"echo","params":{"value":"ok"}}`,
		`{"jsonrpc":"2.0","method":"missing"}`,
		`[{"jsonrpc":"2.0","method":"echo"},{"jsonrpc":"2.0","method":"missing"}]`,
	} {
		if response := s.Handle(t.Context(), []byte(raw)); response != nil {
			t.Fatalf("notification replied: %s", response)
		}
	}
	raw := s.Handle(t.Context(), []byte(`[1,{"jsonrpc":"2.0","method":"missing"},{"jsonrpc":"2.0","method":"echo","params":{"value":"ok"},"id":"yes"}]`))
	var responses []rpcResponse
	err := json.Unmarshal(raw, &responses)
	if err != nil || len(responses) != 2 || responses[0].Error.Code != -32600 || string(responses[1].ID) != `"yes"` {
		t.Fatalf("bad batch: %s / %v", raw, err)
	}
}

func TestProtocolErrorMappingDoesNotLeakCause(t *testing.T) {
	for _, tc := range []struct {
		code   ErrorCode
		number int
	}{{CodeUnknownMethod, -32601}, {CodeInvalidParams, -32602}, {CodeNotFound, -32004}, {CodeConflict, -32009}, {CodeInternal, -32603}} {
		mapped := protocolError(&Error{Code: tc.code, Message: "public", Cause: &Error{Message: "private password"}})
		if mapped.Code != tc.number {
			t.Fatal(mapped)
		}
		raw := encodeRPC(nil, nil, mapped)
		if strings.Contains(string(raw), "private") {
			t.Fatal("cause leaked")
		}
	}
}

func TestProtocolMethodNamesAreReserved(t *testing.T) {
	s := New()
	for _, name := range []string{"initialize", "server/unsubscribe", "rpc.internal"} {
		err := Register(s, Method[echoInput, echoInput]{Name: name}, echoHandler{}.call)
		if err == nil {
			t.Fatalf("reserved name accepted: %s", name)
		}
	}
}
