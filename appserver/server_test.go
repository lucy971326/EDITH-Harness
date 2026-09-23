package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/hooks"
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/persist"
	machinelocal "harness/plugins/machine/local"
)

func TestHookSettingsMethodsBindAndValidate(t *testing.T) {
	h := host.NewHost()
	if err := h.Install(machinelocal.New()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	filesystem, err := host.Resolve[machine.FileSystem](h, "machine")
	if err != nil {
		t.Fatal(err)
	}
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := hooks.NewService(files, filesystem)
	if err != nil {
		t.Fatal(err)
	}
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	if err := server.BindHooks(service); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method string
		input  string
	}{
		{"hooks/read", `{"workspace":""}`},
		{"hooks/save", `{"scope":"global","workspace":"","hash":"","hooks":[]}`},
	} {
		if _, err := server.Call(context.Background(), request.method, json.RawMessage(request.input)); err != nil {
			t.Fatalf("%s: %v", request.method, err)
		}
	}
	scope, err := files.Scope("hooks")
	if err != nil {
		t.Fatal(err)
	}
	if err := scope.Write("settings.json", []byte("bad JSON")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Call(context.Background(), "hooks/read", json.RawMessage(`{"workspace":""}`)); err != nil {
		t.Fatalf("hooks/read must show invalid configuration: %v", err)
	}
}

// 数据。覆盖必填、可选、枚举、数组、引用与时间的测试契约。
type contractInput struct {
	Name  string         `json:"name" jsonschema:"minLength=1"`
	Mode  string         `json:"mode" jsonschema:"enum=read,enum=write"`
	Note  string         `json:"note,omitempty"`
	Items []contractItem `json:"items"`
	At    time.Time      `json:"at"`
}

// 数据。嵌套契约必须拒绝未知字段。
type contractItem struct {
	Value string `json:"value"`
}

// 数据。测试输出校验。
type contractOutput struct {
	Value string `json:"value" jsonschema:"minLength=1"`
}

const sample = "test/call"

const validInput = `{"name":"ok","mode":"read","items":[{"value":"x"}],"at":"2026-09-07T10:00:00Z"}`

type sessionCallInput struct {
	SessionID string `json:"sessionID" jsonschema:"minLength=1"`
	Name      string `json:"name"`
}

func TestContractRegistration(t *testing.T) {
	s := New()
	handler := func(context.Context, contractInput) (contractOutput, error) { return contractOutput{Value: "ok"}, nil }
	err := Register(s, sample, handler)
	if err != nil {
		t.Fatal(err)
	}
	err = Register(s, sample, handler)
	if err == nil {
		t.Fatal("duplicate method accepted")
	}
	err = Register[contractInput, contractOutput](s, "empty-handler", nil)
	if err == nil {
		t.Fatal("nil handler accepted")
	}
	err = Register(s, "", handler)
	if err == nil {
		t.Fatal("empty name accepted")
	}
	err = Register(s, "scalar", func(context.Context, string) (contractOutput, error) { return contractOutput{}, nil })
	if err == nil {
		t.Fatal("non-object contract accepted")
	}
	// 无效正则是一个编译期契约错误，不应等到调用时暴露。
	err = Register(s, "invalid", func(context.Context, invalidPattern) (contractOutput, error) { return contractOutput{}, nil })
	if err == nil {
		t.Fatal("invalid schema accepted")
	}
	_, err = s.Call(context.Background(), sample, json.RawMessage(validInput))
	if err != nil {
		t.Fatal(err)
	}
	err = Register(s, "late", handler)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Call(context.Background(), "missing", json.RawMessage(validInput))
	assertCode(t, err, CodeUnknownMethod)
}

// 数据。故意无效的契约。
type invalidPattern struct {
	Text string `json:"text" jsonschema:"pattern=["`
}

func TestInputAndOutputValidation(t *testing.T) {
	s := New()
	var calls atomic.Int32
	err := Register(s, sample, func(_ context.Context, input contractInput) (contractOutput, error) {
		calls.Add(1)
		if input.Name == "bad-output" {
			return contractOutput{}, nil
		}
		if input.Name == "failure" {
			return contractOutput{}, errors.New("private path")
		}
		if input.Name == "conflict" {
			return contractOutput{}, &Error{Code: CodeConflict, Message: "busy"}
		}
		return contractOutput{Value: input.Name}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`null`, `[]`, `"text"`, `{}`, validInput + `{}`, `{"name":"ok","mode":"other","items":[],"at":"2026-09-07T10:00:00Z"}`,
		`{"name":"","mode":"read","items":[],"at":"2026-09-07T10:00:00Z"}`,
		`{"name":"ok","mode":"read","items":null,"at":"2026-09-07T10:00:00Z"}`,
		`{"name":"ok","mode":"read","items":[{"value":"x","extra":true}],"at":"2026-09-07T10:00:00Z"}`,
		`{"name":"ok","mode":"read","items":[],"at":"not-time"}`,
		`{"name":"ok","mode":"read","items":[],"at":"2026-09-07T10:00:00Z","extra":true}`,
	} {
		_, err = s.Call(context.Background(), sample, json.RawMessage(raw))
		assertCode(t, err, CodeInvalidParams)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached handler")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Call(cancelled, sample, json.RawMessage(validInput))
	assertCode(t, err, CodeInternal)
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatal("cancelled request reached handler or lost cause")
	}
	for _, name := range []string{"ok", "bad-output", "failure", "conflict"} {
		params := contractInput{Name: name, Mode: "write", Note: "optional", Items: []contractItem{}, At: time.Now()}
		raw, marshalErr := json.Marshal(params)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		_, err = s.Call(context.Background(), sample, raw)
		switch name {
		case "ok":
			if err != nil {
				t.Fatal(err)
			}
		case "conflict":
			assertCode(t, err, CodeConflict)
		default:
			assertCode(t, err, CodeInternal)
		}
	}
}

func TestConcurrentCalls(t *testing.T) {
	s := New()
	err := Register(s, sample, func(_ context.Context, input contractInput) (contractOutput, error) {
		return contractOutput{Value: input.Name}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := s.Call(context.Background(), sample, json.RawMessage(validInput))
			if callErr != nil {
				t.Error(callErr)
			}
		}()
	}
	wg.Wait()
}

func TestSessionWritesAreOrderedButControlCallIsDirect(t *testing.T) {
	server := New()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	controlCalled := make(chan struct{})
	err := registerSession(server, "session/write", func(input sessionCallInput) string { return input.SessionID }, func(_ context.Context, input sessionCallInput) (struct{}, error) {
		if input.Name == "first" {
			close(firstStarted)
			<-releaseFirst
		} else {
			close(secondStarted)
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = Register(server, "session/control", func(context.Context, sessionCallInput) (struct{}, error) {
		close(controlCalled)
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, callErr := server.Call(context.Background(), "session/write", json.RawMessage(`{"sessionID":"a","name":"first"}`))
		firstDone <- callErr
	}()
	<-firstStarted
	secondDone := make(chan error, 1)
	go func() {
		_, callErr := server.Call(context.Background(), "session/write", json.RawMessage(`{"sessionID":"a","name":"second"}`))
		secondDone <- callErr
	}()

	_, err = server.Call(context.Background(), "session/control", json.RawMessage(`{"sessionID":"a","name":"stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-controlCalled:
	case <-time.After(time.Second):
		t.Fatal("control call waited behind the session queue")
	}
	select {
	case <-secondStarted:
		t.Fatal("same-session write overtook the first")
	default:
	}
	close(releaseFirst)
	if err = <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err = <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestCloseRejectsNewCallsAndWaitsForAcceptedCall(t *testing.T) {
	s := New()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	err := Register(s, sample, func(context.Context, contractInput) (contractOutput, error) {
		close(started)
		<-release
		return contractOutput{Value: "ok"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Call(context.Background(), sample, json.RawMessage(validInput)); done <- err }()
	<-started
	closed := make(chan struct{})
	go func() { _ = s.Close(); close(closed) }()
	// 同包检查关闭准入完成，避免依赖调度延迟来建立时序。
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.lifecycle.mu.Lock()
		stopped := s.lifecycle.closed
		s.lifecycle.mu.Unlock()
		if stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not close admission")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-closed:
		t.Fatal("Close did not wait")
	default:
	}
	_, err = s.Call(context.Background(), sample, json.RawMessage(validInput))
	assertCode(t, err, CodeConflict)
	releaseOnce.Do(func() { close(release) })
	err = <-done
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestClosedServerCannotReopen(t *testing.T) {
	s := New()
	err := s.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = Register(s, sample, func(context.Context, contractInput) (contractOutput, error) { return contractOutput{Value: "ok"}, nil })
	if err == nil {
		t.Fatal("closed server accepted registration")
	}
	_, err = s.Call(context.Background(), sample, json.RawMessage(validInput))
	assertCode(t, err, CodeConflict)
}

func assertCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
