package machinelocal

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness/kernel/machine"
	"harness/kernel/permissions"
)

func TestProcessOutputIsIncremental(t *testing.T) {
	m := newTestLocal(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	// 普通管道不接收标准输入；创建信号文件后才允许输出第二段。
	first, err := m.AgentExec(ctx, permissions.Policy{Unrestricted: true}, machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     dir,
		Argv:    []string{"bash", "--noprofile", "--norc", "-c", "printf first; while [ ! -f continue ]; do sleep 0.05; done; printf second"},
		Wait:    50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	initial := string(first.Output)
	for !first.Exited && len(initial) < len("first") {
		first, err = m.AgentInteract(ctx, machine.ProcessInteraction{
			OwnerID: "session-a", ProcessID: first.ProcessID, Wait: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		initial += string(first.Output)
	}
	if first.Exited || initial != "first" {
		t.Fatalf("first output = %#v", first)
	}
	err = os.WriteFile(filepath.Join(dir, "continue"), nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	last, err := m.AgentInteract(ctx, machine.ProcessInteraction{
		OwnerID:   "session-a",
		ProcessID: first.ProcessID,
		Wait:      50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	remaining := string(last.Output)
	for !last.Exited {
		last, err = m.AgentInteract(ctx, machine.ProcessInteraction{
			OwnerID: "session-a", ProcessID: last.ProcessID, Wait: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		remaining += string(last.Output)
	}
	if last.ExitCode != 0 {
		t.Fatalf("last output = %#v", last)
	}
	if remaining != "second" {
		t.Fatalf("last output = %q, want only new output", remaining)
	}
}

func TestProcessTTYAcceptsInput(t *testing.T) {
	m := newTestLocal(t)
	started, err := m.AgentExec(context.Background(), permissions.Policy{Unrestricted: true}, machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     t.TempDir(),
		Argv:    []string{"bash", "-lic", `read value; printf 'got:%s\n' "$value"`},
		TTY:     true,
		Wait:    500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Exited {
		t.Fatalf("process exited before input: %#v", started)
	}

	finished, err := m.AgentInteract(context.Background(), machine.ProcessInteraction{
		OwnerID:   "session-a",
		ProcessID: started.ProcessID,
		Chars:     []byte("hello\n"),
		Wait:      3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !finished.Exited || !strings.Contains(string(finished.Output), "got:hello") {
		t.Fatalf("terminal output = %#v", finished)
	}
}

func TestTerminalHandleStreamsInputAndResizes(t *testing.T) {
	m := newTestLocal(t)
	process, err := m.StartTerminal(machine.TerminalRequest{
		Dir:  t.TempDir(),
		Argv: []string{"bash", "-lic", `read value; printf 'got:%s\n' "$value"`},
		Rows: 20,
		Cols: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Resize(30, 90); err != nil {
		t.Fatal(err)
	}
	if err = process.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	var output []byte
	for chunk := range process.Output() {
		output = append(output, chunk...)
	}
	exitCode, err := process.Wait(t.Context())
	if err != nil || exitCode != 0 || !strings.Contains(string(output), "got:hello") {
		t.Fatalf("exit=%d error=%v output=%q", exitCode, err, output)
	}
}

func TestProcessOwnerIsPrivate(t *testing.T) {
	dir := t.TempDir()
	m := newTestLocal(t)
	started, err := m.AgentExec(context.Background(), permissions.Policy{Unrestricted: true}, machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     dir,
		Argv:    []string{"bash", "-lc", "sleep 2"},
		Wait:    250 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.AgentInteract(context.Background(), machine.ProcessInteraction{
		OwnerID:   "session-b",
		ProcessID: started.ProcessID,
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("wrong-owner error = %v", err)
	}
}

func TestCanceledInitialExecRemovesProcess(t *testing.T) {
	m := newTestLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.AgentExec(ctx, permissions.Policy{Unrestricted: true}, machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     t.TempDir(),
		Argv:    []string{"bash", "-lc", "sleep 10"},
		Wait:    time.Second,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec() error = %v, want context.Canceled", err)
	}
	m.mu.Lock()
	count := len(m.processes)
	m.mu.Unlock()
	if count != 0 {
		t.Fatalf("tracked processes = %d, want 0", count)
	}
}

func TestCloseStopsRunningProcess(t *testing.T) {
	dir := t.TempDir()
	m := newTestLocal(t)
	started, err := m.AgentExec(context.Background(), permissions.Policy{Unrestricted: true}, machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     dir,
		Argv:    []string{"bash", "-lc", "sleep 30"},
		Wait:    250 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Exited {
		t.Fatal("process exited before Close")
	}

	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestHeadTailBufferCapsOutput(t *testing.T) {
	var buffer headTailBuffer
	input := append(bytes.Repeat([]byte("a"), maxProcessOutput/2), bytes.Repeat([]byte("x"), 100)...)
	input = append(input, bytes.Repeat([]byte("b"), maxProcessOutput/2)...)
	buffer.append(input)

	output, omitted := buffer.take()
	if len(output) != maxProcessOutput || omitted != 100 {
		t.Fatalf("len = %d, omitted = %d", len(output), omitted)
	}
	if !bytes.Equal(output[:16], bytes.Repeat([]byte("a"), 16)) ||
		!bytes.Equal(output[len(output)-16:], bytes.Repeat([]byte("b"), 16)) {
		t.Fatal("buffer did not preserve head and tail")
	}
}
