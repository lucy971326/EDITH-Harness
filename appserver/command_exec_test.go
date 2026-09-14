package appserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"harness/kernel/machine"

	"github.com/coder/websocket"
)

type fakeTerminalSystem struct {
	process *fakeTerminalProcess
}

func (f fakeTerminalSystem) StartTerminal(machine.TerminalRequest) (machine.TerminalProcess, error) {
	return f.process, nil
}

type fakeTerminalProcess struct {
	output     chan []byte
	done       chan struct{}
	terminated chan struct{}
	finishOnce sync.Once
	stopOnce   sync.Once

	mu       sync.Mutex
	exitCode int
}

func newFakeTerminalProcess() *fakeTerminalProcess {
	return &fakeTerminalProcess{
		output:     make(chan []byte, 1),
		done:       make(chan struct{}),
		terminated: make(chan struct{}),
	}
}

func (p *fakeTerminalProcess) Output() <-chan []byte { return p.output }
func (p *fakeTerminalProcess) Write([]byte) error    { return nil }
func (p *fakeTerminalProcess) CloseInput() error     { return nil }
func (p *fakeTerminalProcess) Resize(int, int) error { return nil }
func (p *fakeTerminalProcess) Terminate() error {
	p.stopOnce.Do(func() { close(p.terminated) })
	p.finish(1)
	return nil
}
func (p *fakeTerminalProcess) Wait(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.exitCode, nil
	}
}

func (p *fakeTerminalProcess) finish(exitCode int) {
	p.finishOnce.Do(func() {
		p.mu.Lock()
		p.exitCode = exitCode
		p.mu.Unlock()
		close(p.output)
		close(p.done)
	})
}

func TestCommandExecDisconnectTerminatesProcess(t *testing.T) {
	process := newFakeTerminalProcess()
	server := New()
	if err := server.BindCommandExec(fakeTerminalSystem{process: process}); err != nil {
		t.Fatal(err)
	}
	_, url := startTestSocket(t, server)
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)

	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  commandExecMethod,
		"params": CommandExecParams{
			ProcessID: "terminal-1",
			CWD:       t.TempDir(),
			Size:      CommandExecTerminalSize{Rows: 24, Cols: 80},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ws.Write(t.Context(), websocket.MessageText, request); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		server.commandExec.mu.Lock()
		started := len(server.commandExec.sessions) == 1
		server.commandExec.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal did not start")
		}
		time.Sleep(time.Millisecond)
	}
	_ = ws.CloseNow()
	select {
	case <-process.terminated:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not terminate terminal")
	}
}

func TestCommandExecStreamsBeforeFinalResponse(t *testing.T) {
	process := newFakeTerminalProcess()
	server := New()
	err := server.BindCommandExec(fakeTerminalSystem{process: process})
	if err != nil {
		t.Fatal(err)
	}
	_, url := startTestSocket(t, server)
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)

	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  commandExecMethod,
		"params": CommandExecParams{
			ProcessID: "terminal-1",
			CWD:       t.TempDir(),
			Size:      CommandExecTerminalSize{Rows: 24, Cols: 80},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ws.Write(t.Context(), websocket.MessageText, request)
	if err != nil {
		t.Fatal(err)
	}
	process.output <- []byte("prompt")
	process.finish(7)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var notification struct {
		Method string                 `json:"method"`
		Params CommandExecOutputDelta `json:"params"`
	}
	if err = json.Unmarshal(raw, &notification); err != nil {
		t.Fatal(err)
	}
	if notification.Method != commandExecOutputMethod || notification.Params.DeltaBase64 != base64.StdEncoding.EncodeToString([]byte("prompt")) {
		t.Fatalf("unexpected first message: %s", raw)
	}

	_, raw, err = ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var response rpcResponse
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || string(response.Result) != `{"exitCode":7}` {
		t.Fatalf("unexpected final response: %s", raw)
	}
}
