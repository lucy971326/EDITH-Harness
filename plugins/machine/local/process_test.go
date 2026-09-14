package machinelocal

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"harness/kernel/machine"
)

func TestProcessOutputIsIncremental(t *testing.T) {
	m := newTestLocal(t)
	first, err := m.Exec(context.Background(), machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     t.TempDir(),
		Argv:    []string{"bash", "-lc", "printf first; sleep 1; printf second"},
		Wait:    500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Exited || !strings.Contains(string(first.Output), "first") {
		t.Fatalf("first output = %#v", first)
	}

	last, err := m.Interact(context.Background(), machine.ProcessInteraction{
		OwnerID:   "session-a",
		ProcessID: first.ProcessID,
		Wait:      2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !last.Exited || last.ExitCode != 0 {
		t.Fatalf("last output = %#v", last)
	}
	if strings.Contains(string(last.Output), "first") || !strings.Contains(string(last.Output), "second") {
		t.Fatalf("last output = %q, want only new output", last.Output)
	}
}

func TestProcessTTYAcceptsInput(t *testing.T) {
	m := newTestLocal(t)
	started, err := m.Exec(context.Background(), machine.ProcessRequest{
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

	finished, err := m.Interact(context.Background(), machine.ProcessInteraction{
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

func TestProcessOwnerIsPrivate(t *testing.T) {
	dir := t.TempDir()
	m := newTestLocal(t)
	started, err := m.Exec(context.Background(), machine.ProcessRequest{
		OwnerID: "session-a",
		Dir:     dir,
		Argv:    []string{"bash", "-lc", "sleep 2"},
		Wait:    250 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Interact(context.Background(), machine.ProcessInteraction{
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

	_, err := m.Exec(ctx, machine.ProcessRequest{
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
	started, err := m.Exec(context.Background(), machine.ProcessRequest{
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
	go func() { done <- m.close() }()
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
