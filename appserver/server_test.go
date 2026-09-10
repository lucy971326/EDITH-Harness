package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

var sample = Method[contractInput, contractOutput]{Name: "test/call"}

const validInput = `{"name":"ok","mode":"read","items":[{"value":"x"}],"at":"2026-09-07T10:00:00Z"}`

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
	err = Register(s, Method[contractInput, contractOutput]{Name: "empty-handler"}, nil)
	if err == nil {
		t.Fatal("nil handler accepted")
	}
	err = Register(s, Method[contractInput, contractOutput]{}, handler)
	if err == nil {
		t.Fatal("empty name accepted")
	}
	_, _, err = compileMethod(Method[string, contractOutput]{Name: "scalar"})
	if err == nil {
		t.Fatal("non-object contract accepted")
	}
	// 无效正则是一个编译期契约错误，不应等到调用时暴露。
	_, _, err = compileMethod(Method[invalidPattern, contractOutput]{Name: "invalid"})
	if err == nil {
		t.Fatal("invalid schema accepted")
	}
	_, err = s.Call(context.Background(), sample.Name, json.RawMessage(validInput))
	if err != nil {
		t.Fatal(err)
	}
	err = Register(s, Method[contractInput, contractOutput]{Name: "late"}, handler)
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
		_, err = s.Call(context.Background(), sample.Name, json.RawMessage(raw))
		assertCode(t, err, CodeInvalidParams)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached handler")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Call(cancelled, sample.Name, json.RawMessage(validInput))
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
		_, err = s.Call(context.Background(), sample.Name, raw)
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
			_, callErr := s.Call(context.Background(), sample.Name, json.RawMessage(validInput))
			if callErr != nil {
				t.Error(callErr)
			}
		}()
	}
	wg.Wait()
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
	go func() { _, err := s.Call(context.Background(), sample.Name, json.RawMessage(validInput)); done <- err }()
	<-started
	closed := make(chan struct{})
	go func() { _ = s.Close(); close(closed) }()
	// 同包检查关闭准入完成，避免依赖调度延迟来建立时序。
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.RLock()
		stopped := s.closed
		s.mu.RUnlock()
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
	_, err = s.Call(context.Background(), sample.Name, json.RawMessage(validInput))
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
	_, err = s.Call(context.Background(), sample.Name, json.RawMessage(validInput))
	assertCode(t, err, CodeConflict)
}

func assertCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
