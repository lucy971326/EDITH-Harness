package appserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string            `json:"jsonrpc"`
	Method  string            `json:"method"`
	Params  subscriptionEvent `json:"params"`
}

type subscriptionEvent struct {
	SubscriptionID string `json:"subscriptionID"`
	Event          any    `json:"event"`
}

func encodeNotification(method, subscriptionID string, event any) ([]byte, error) {
	return json.Marshal(rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  subscriptionEvent{SubscriptionID: subscriptionID, Event: event},
	})
}

// Handle 处理一条 JSON 消息；支持批量请求，通知返回 nil。这里不读写网络。
func (s *RPCServer) Handle(ctx context.Context, raw []byte) []byte {
	if !json.Valid(raw) {
		return encodeRPC(nil, nil, &RPCError{Code: -32700, Message: "Parse error"})
	}
	raw = bytes.TrimSpace(raw)
	if raw[0] != '[' {
		return s.handleRequest(ctx, raw)
	}
	var batch []json.RawMessage
	_ = json.Unmarshal(raw, &batch)
	if len(batch) == 0 {
		return encodeRPC(nil, nil, &RPCError{Code: -32600, Message: "Invalid Request"})
	}
	responses := make([]json.RawMessage, 0, len(batch))
	for _, item := range batch {
		response := s.handleRequest(ctx, item)
		if response != nil {
			responses = append(responses, response)
		}
	}
	if len(responses) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(responses)
	return encoded
}

func (s *RPCServer) handleRequest(ctx context.Context, raw []byte) []byte {
	var fields map[string]json.RawMessage
	err := json.Unmarshal(raw, &fields)
	if err != nil || fields == nil {
		return encodeRPC(nil, nil, &RPCError{Code: -32600, Message: "Invalid Request"})
	}
	var version string
	err = json.Unmarshal(fields["jsonrpc"], &version)
	if err != nil || version != "2.0" {
		return encodeRPC(nil, nil, &RPCError{Code: -32600, Message: "Invalid Request"})
	}
	var method string
	err = json.Unmarshal(fields["method"], &method)
	if err != nil || bytes.Equal(fields["method"], []byte("null")) {
		return encodeRPC(nil, nil, &RPCError{Code: -32600, Message: "Invalid Request"})
	}
	id, hasID := fields["id"]
	if !validID(id) {
		return encodeRPC(nil, nil, &RPCError{Code: -32600, Message: "Invalid Request"})
	}
	params := fields["params"]
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if params[0] != '{' && params[0] != '[' {
		if !hasID {
			return nil
		}
		return encodeRPC(id, nil, &RPCError{Code: -32602, Message: "Invalid params"})
	}
	result, rpcErr := s.dispatch(ctx, method, params, hasID)
	if !hasID {
		return nil
	}
	return encodeRPC(id, result, rpcErr)
}

func validID(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || raw[0] == '"' {
		return true
	}
	return raw[0] == '-' || raw[0] >= '0' && raw[0] <= '9'
}

func (s *RPCServer) dispatch(ctx context.Context, method string, params json.RawMessage, hasID bool) (json.RawMessage, *RPCError) {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return nil, &RPCError{Code: -32009, Message: "Server is not accepting calls", Data: CodeConflict}
	}
	// 连接握手；直接使用 Handle 的进程内调用可以不携带连接。
	c, _ := ctx.Value(connectionKey{}).(*Connection)
	if c != nil && !c.initialized.Load() {
		if method != "initialize" || !hasID {
			return nil, &RPCError{Code: -32001, Message: "Initialize first"}
		}
		var input InitializeParams
		err := decodeParams(params, &input)
		if err != nil || input.ProtocolVersion != 1 {
			return nil, &RPCError{Code: -32001, Message: "Initialization rejected"}
		}
		c.initialized.Store(true)
		c.initializeTimer.Stop()
		result, _ := json.Marshal(InitializeResult{ProtocolVersion: 1})
		return result, nil
	}
	// 协议内建操作。
	if method == "initialize" {
		return nil, &RPCError{Code: -32002, Message: "Connection already initialized or unavailable"}
	}
	if method == "server/unsubscribe" {
		var input struct {
			SubscriptionID string `json:"subscriptionID"`
		}
		err := decodeParams(params, &input)
		if c == nil || err != nil || input.SubscriptionID == "" {
			return nil, &RPCError{Code: -32602, Message: "Invalid params or connection"}
		}
		c.unsubscribe(input.SubscriptionID)
		return json.RawMessage(`{}`), nil
	}
	// 已登记的产品方法。
	result, err := s.Call(ctx, method, params)
	return result, protocolError(err)
}

func decodeParams(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func protocolError(err error) *RPCError {
	if err == nil {
		return nil
	}
	var public *Error
	if !errors.As(err, &public) {
		return &RPCError{Code: -32603, Message: "Internal error"}
	}
	code := -32603
	switch public.Code {
	case CodeUnknownMethod:
		code = -32601
	case CodeInvalidParams:
		code = -32602
	case CodeNotFound:
		code = -32004
	case CodeConflict:
		code = -32009
	}
	message := public.Message
	if code == -32603 {
		message = "Internal error"
	}
	return &RPCError{Code: code, Message: message, Data: public.Code}
}

func encodeRPC(id, result json.RawMessage, rpcErr *RPCError) []byte {
	if id == nil {
		id = json.RawMessage(`null`)
	}
	if rpcErr == nil && result == nil {
		result = json.RawMessage(`null`)
	}
	encoded, _ := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: rpcErr})
	return encoded
}
