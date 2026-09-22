package machinelocal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"harness/kernel/machine"
	"harness/kernel/permissions"
)

func TestMain(m *testing.M) {
	handled, err := RunFileWorker(os.Args[1:], os.Stdin, os.Stdout)
	if handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 4 && os.Args[1] == "--sandbox-probe" {
		if os.Args[2] == "unixgram" {
			fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
			if err != nil {
				os.Exit(1)
			}
			_, err = unix.SendmsgN(fd, []byte("probe"), nil, &unix.SockaddrUnix{Name: os.Args[3]}, 0)
			_ = unix.Close(fd)
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}

		conn, err := net.DialTimeout(os.Args[2], os.Args[3], time.Second)
		if err != nil {
			os.Exit(1)
		}
		_ = conn.Close()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLinuxAgentSandbox(t *testing.T) {
	if _, err := os.Stat("/usr/bin/bwrap"); err != nil {
		t.Skip("requires system bubblewrap")
	}
	m, err := newLocal()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.close() })
	root := t.TempDir()
	workspace := filepath.Join(root, "project")
	err = os.Mkdir(workspace, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	err = os.WriteFile(outside, []byte("original"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(outside, filepath.Join(workspace, "link"))
	if err != nil {
		t.Fatal(err)
	}
	policy := permissions.Policy{WriteRoots: []string{workspace}}
	run := func(policy permissions.Policy, argv ...string) machine.ProcessOutput {
		t.Helper()
		output, err := m.AgentExec(context.Background(), policy, machine.ProcessRequest{OwnerID: "session", Dir: workspace, Argv: argv, Wait: 2 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if !output.Exited {
			t.Fatalf("unexpected live process: %+v", output)
		}
		return output
	}
	output := run(policy, "/bin/true")
	if output.ExitCode != 0 {
		t.Fatalf("sandbox unavailable: %s", output.Output)
	}
	for _, test := range []struct {
		name, command string
		allowed       bool
	}{
		{"workspace", "echo allowed > file", true},
		{"outside", "echo bad > ../outside", false},
		{"symlink", "echo bad > link", false},
		{"metadata", "echo bad > .git/config", false},
		{"missing metadata", "echo bad > .agents/config", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := run(policy, "bash", "-c", test.command)
			if (result.ExitCode == 0) != test.allowed {
				t.Fatalf("exit=%d output=%s", result.ExitCode, result.Output)
			}
		})
	}
	if run(permissions.Policy{}, "bash", "-c", "echo bad > file").ExitCode == 0 {
		t.Fatal("read-only write succeeded")
	}
	if run(permissions.Policy{Unrestricted: true}, "bash", "-c", "echo full > ../outside").ExitCode != 0 {
		t.Fatal("full access failed")
	}
	// 用户直接操作路径不受 Agent 的 Policy 约束。
	err = m.WriteFile(outside, []byte("user"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", ".agents", ".harness"} {
		_, err = os.Lstat(filepath.Join(workspace, name))
		if !os.IsNotExist(err) {
			t.Fatalf("placeholder leaked: %s, %v", name, err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"tcp", "unix"} {
		address := "127.0.0.1:0"
		if network == "unix" {
			address = filepath.Join(root, "host.sock")
		}
		listener, err := net.Listen(network, address)
		if err != nil {
			t.Fatal(err)
		}
		address = listener.Addr().String()
		// 对照组证明目标可达，沙箱组才应被阻断。
		if run(permissions.Policy{Unrestricted: true}, executable, "--sandbox-probe", network, address).ExitCode != 0 {
			t.Fatal("probe baseline failed")
		}
		if run(policy, executable, "--sandbox-probe", network, address).ExitCode == 0 {
			t.Fatalf("sandbox connected to host %s", network)
		}
		_ = listener.Close()
	}
	// 无 connect 的 Unix datagram 也必须被阻断。
	datagramPath := filepath.Join(root, "datagram.sock")
	datagram, err := net.ListenPacket("unixgram", datagramPath)
	if err != nil {
		t.Fatal(err)
	}
	defer datagram.Close()
	if run(permissions.Policy{Unrestricted: true}, executable, "--sandbox-probe", "unixgram", datagramPath).ExitCode != 0 {
		t.Fatal("datagram baseline failed")
	}
	if run(policy, executable, "--sandbox-probe", "unixgram", datagramPath).ExitCode == 0 {
		t.Fatal("Unix datagram bypassed sandbox")
	}
	content := "patched"
	file := filepath.Join(workspace, "patch.txt")
	result, err := m.AgentApplyChanges(context.Background(), policy, []machine.FileChange{{Path: file, Content: &content}, {Path: outside, Content: &content}})
	if err == nil || result.Completed != 0 || !result.Exact {
		t.Fatalf("preflight: %+v %v", result, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("preflight partially wrote")
	}
	result, err = m.AgentApplyChanges(context.Background(), policy, []machine.FileChange{{Path: file, Content: &content}})
	if err != nil || result.Completed != 1 || !result.Exact {
		t.Fatalf("patch: %+v %v", result, err)
	}
	second := filepath.Join(workspace, "second")
	result, err = m.AgentApplyChanges(context.Background(), policy, []machine.FileChange{{Path: second, Content: &content}, {Path: file, ExpectedHash: "wrong", Content: &content}})
	if err == nil || result.Completed != 1 || !result.Exact {
		t.Fatalf("partial conflict: %+v %v", result, err)
	}
	// 进程 ID 交付后不再接受 Policy；后续输入仍在原沙箱中。
	started, err := m.AgentExec(context.Background(), policy, machine.ProcessRequest{OwnerID: "session", Dir: workspace, Argv: []string{"bash", "-c", "read line; echo bad > ../outside; echo still-alive"}, TTY: true, Wait: 20 * time.Millisecond})
	if err != nil || started.Exited {
		t.Fatalf("start PTY: %+v %v", started, err)
	}
	_, err = m.AgentInteract(context.Background(), machine.ProcessInteraction{OwnerID: "other", ProcessID: started.ProcessID, Chars: []byte("go\n")})
	if err == nil {
		t.Fatal("cross-session interaction accepted")
	}
	finished, err := m.AgentInteract(context.Background(), machine.ProcessInteraction{OwnerID: "session", ProcessID: started.ProcessID, Chars: []byte("go\n"), Wait: time.Second})
	if err != nil || !finished.Exited || !strings.Contains(string(finished.Output), "still-alive") {
		t.Fatalf("interact: %+v %v", finished, err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "user" {
		t.Fatalf("outside changed: %q %v", data, err)
	}
}

func TestAgentFileLocking(t *testing.T) {
	t.Run("stable keys and cancellable wait", func(t *testing.T) {
		m := newTestLocal(t)
		dir := t.TempDir()
		a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
		for _, path := range []string{a, b} {
			err := os.WriteFile(path, []byte("old"), 0o600)
			if err != nil {
				t.Fatal(err)
			}
		}
		value := "new"
		batch := []machine.FileChange{{Path: a, ExpectedHash: fileHash([]byte("old")), Content: &value}, {Path: b, ExpectedHash: fileHash([]byte("old")), Content: &value}}
		type reply struct {
			result machine.FileCommit
			err    error
		}
		done := make(chan reply, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		unlock := m.fileLocks.lock(a)
		defer func() {
			if unlock != nil {
				unlock()
			}
		}()
		go func() {
			result, err := m.AgentApplyChanges(ctx, permissions.Policy{Unrestricted: true}, batch)
			done <- reply{result, err}
		}()
		// 第一把锁出现等待者，说明整批 key 已经计算完成。
		deadline := time.Now().Add(time.Second)
		for {
			m.fileLocks.mu.Lock()
			waiting := m.fileLocks.items[a].refs == 2
			m.fileLocks.mu.Unlock()
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("batch did not wait on first lock")
			}
			time.Sleep(time.Millisecond)
		}
		err := os.Remove(b)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Symlink(a, b)
		if err != nil {
			t.Fatal(err)
		}
		unlock()
		unlock = nil
		select {
		case reply := <-done:
			if reply.err == nil || reply.result.Completed != 1 {
				t.Fatalf("expected second-file conflict: %+v", reply)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("path alias change deadlocked the batch")
		}

		unlock = m.fileLocks.lock(a)
		cancel()
		// 已取消的调用不能无限等待编辑器持有的路径锁。
		go func() {
			result, err := m.AgentApplyChanges(ctx, permissions.Policy{Unrestricted: true}, batch[:1])
			done <- reply{result, err}
		}()
		select {
		case reply := <-done:
			if !errors.Is(reply.err, context.Canceled) {
				t.Fatalf("cancelled lock wait: %v", reply.err)
			}
		case <-time.After(time.Second):
			t.Fatal("cancelled batch still waits for path lock")
		}
	})

	t.Run("full access IO does not hold machine lock", func(t *testing.T) {
		m := newTestLocal(t)
		fifo := filepath.Join(t.TempDir(), "pipe")
		err := unix.Mkfifo(fifo, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		finished := false
		// 失败时保持 FIFO 打开直到调用返回，确保回归用例本身也能收尾。
		defer func() {
			cancel()
			if finished {
				return
			}
			fd, openErr := unix.Open(fifo, unix.O_RDWR|unix.O_NONBLOCK, 0)
			if openErr == nil {
				defer unix.Close(fd)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}()
		value := "new"
		go func() {
			_, err := m.AgentApplyChanges(ctx, permissions.Policy{Unrestricted: true}, []machine.FileChange{{Path: fifo, ExpectedHash: "old", Content: &value}})
			done <- err
		}()
		started := make(chan struct{})
		go func() {
			for ctx.Err() == nil {
				m.mu.Lock()
				running := len(m.processes) != 0
				m.mu.Unlock()
				if running {
					close(started)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("file IO blocked machine registration lock")
		}
		output, err := m.AgentExec(context.Background(), permissions.Policy{Unrestricted: true}, machine.ProcessRequest{OwnerID: "other", Argv: []string{"/bin/true"}, Wait: time.Second})
		if err != nil || !output.Exited || output.ExitCode != 0 {
			t.Fatalf("unrelated command blocked: %+v %v", output, err)
		}
		cancel()
		select {
		case err := <-done:
			finished = true
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("worker cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("file worker did not stop")
		}
	})
}
