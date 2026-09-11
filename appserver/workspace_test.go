package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelectWorkspaceSuccessCancelAndFailure(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	selectWorkspace = func(context.Context) (string, error) { return workspace, nil }
	raw, err := server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectResult(t, raw, false, filepath.Clean(workspace))

	selectWorkspace = func(context.Context) (string, error) { return "", errWorkspaceCanceled }
	raw, err = server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectResult(t, raw, true, "")

	selectWorkspace = func(context.Context) (string, error) { return "", errors.New("picker unavailable") }
	_, err = server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	assertCode(t, err, CodeInternal)

	selectWorkspace = func(context.Context) (string, error) { return "relative", nil }
	_, err = server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	assertCode(t, err, CodeInternal)

	for _, params := range []string{`null`, `[]`, `{"extra":true}`} {
		_, err = server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(params))
		assertCode(t, err, CodeInvalidParams)
	}
}

func TestSelectWorkspaceDoesNotCreateASession(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}
	if server.harnessProduct != nil {
		t.Fatal("workspace select must not require a product")
	}
	workspace := t.TempDir()
	selectWorkspace = func(context.Context) (string, error) { return workspace, nil }
	raw, err := server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectResult(t, raw, false, filepath.Clean(workspace))
}

func TestSelectWorkspaceCancelDoesNotSurfaceAsRPCError(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}
	selectWorkspace = func(context.Context) (string, error) { return "", nil }
	raw, err := server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSelectResult(t, raw, true, "")
}

func TestSelectWorkspaceConcurrentCalls(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	var calls atomic.Int32
	selectWorkspace = func(context.Context) (string, error) {
		calls.Add(1)
		return workspace, nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := server.Call(context.Background(), selectWorkspaceMethod, json.RawMessage(`{}`))
			if err != nil {
				t.Error(err)
				return
			}
			assertSelectResult(t, raw, false, filepath.Clean(workspace))
		}()
	}
	wg.Wait()
	if calls.Load() != 8 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestSelectWorkspaceOverWebSocket(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	selectWorkspace = func(context.Context) (string, error) { return workspace, nil }
	_, url := startTestSocket(t, server)
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)
	response := socketRequest(t, ws, `{"jsonrpc":"2.0","id":"pick","method":"workspace/select","params":{}}`)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	assertSelectResult(t, response.Result, false, filepath.Clean(workspace))

	selectWorkspace = func(context.Context) (string, error) { return "", errWorkspaceCanceled }
	response = socketRequest(t, ws, `{"jsonrpc":"2.0","id":"cancel","method":"workspace/select","params":{}}`)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	assertSelectResult(t, response.Result, true, "")
}

func TestSelectWorkspaceReturnsCanceledContext(t *testing.T) {
	t.Cleanup(func() { selectWorkspace = chooseWorkspace })
	server := New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.registerWorkspaceSelect()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	selectWorkspace = func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := server.Call(ctx, selectWorkspaceMethod, json.RawMessage(`{}`))
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		assertCode(t, err, CodeInternal)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want canceled context, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("select workspace ignored canceled context")
	}
}

func assertSelectResult(t *testing.T, raw json.RawMessage, canceled bool, workspace string) {
	t.Helper()
	var result SelectWorkspaceResult
	err := json.Unmarshal(raw, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Canceled != canceled || result.Workspace != workspace {
		t.Fatalf("result = %+v, want canceled=%v workspace=%q", result, canceled, workspace)
	}
}
