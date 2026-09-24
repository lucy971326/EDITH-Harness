package machinelocal

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		conn, err := net.DialTimeout(os.Args[2], os.Args[3], time.Second)
		if err != nil {
			os.Exit(1)
		}
		_ = conn.Close()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDarwinAgentSandbox(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "project with spaces")
	err = os.Mkdir(workspace, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	err = os.WriteFile(outside, []byte("original"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(outside, filepath.Join(workspace, "link"))
	if err != nil {
		t.Fatal(err)
	}
	policy := permissions.Policy{WriteRoots: []string{workspace}}
	run := func(policy permissions.Policy, tty bool, argv ...string) machine.ProcessOutput {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		output, err := m.AgentExec(ctx, policy, machine.ProcessRequest{OwnerID: "mac-test", Dir: workspace, Argv: argv, TTY: tty, Wait: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for !output.Exited {
			output, err = m.AgentInteract(ctx, machine.ProcessInteraction{OwnerID: "mac-test", ProcessID: output.ProcessID, Wait: time.Second})
			if err != nil {
				t.Fatal(err)
			}
		}
		return output
	}
	for _, test := range []struct {
		name, command string
		allowed       bool
	}{
		{"launch", "true", true},
		{"workspace", "echo ok > file", true},
		{"outside", "echo bad > ../outside", false},
		{"symlink", "echo bad > link", false},
		{"create protected", "mkdir .git", false},
		{"rename root", "mv \"$PWD\" ../moved", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := run(policy, false, "/bin/sh", "-c", test.command)
			if (output.ExitCode == 0) != test.allowed {
				t.Fatalf("exit=%d output=%s", output.ExitCode, output.Output)
			}
		})
	}
	for _, name := range []string{".git", ".agents", ".harness"} {
		err = os.Mkdir(filepath.Join(workspace, name), 0o700)
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"touch " + name + "/bad", "rmdir " + name, "mv " + name + " moved"} {
			if output := run(policy, false, "/bin/sh", "-c", command); output.ExitCode == 0 {
				t.Fatalf("protected operation succeeded: %s", command)
			}
		}
	}
	if output := run(permissions.Policy{}, false, "/bin/sh", "-c", "touch readonly"); output.ExitCode == 0 {
		t.Fatal("read-only policy allowed writing")
	}
	granted, err := permissions.ApplyDecision(permissions.ApprovalRequest{Current: policy, Requested: permissions.ExtraPermissions{WriteRoots: []string{root}}}, permissions.Decision{Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		policy permissions.Policy
		allow  bool
	}{{granted, true}, {policy, false}} {
		output := run(item.policy, false, "/bin/sh", "-c", "echo changed > ../outside")
		if (output.ExitCode == 0) != item.allow {
			t.Fatalf("per-operation grant: %s", output.Output)
		}
	}
	content := "from worker"
	commit, err := m.AgentApplyChanges(context.Background(), policy, []machine.FileChange{{Path: filepath.Join(workspace, "patch"), Content: &content}})
	if err != nil || commit.Completed != 1 {
		t.Fatalf("file worker: %+v %v", commit, err)
	}
	if output := run(policy, true, "/bin/sh", "-c", "test -t 0 && test -t 1"); output.ExitCode != 0 {
		t.Fatalf("PTY: %s", output.Output)
	}
	// 同一真实目录的系统别名应可启动；用户自行添加的中间链接不能授权。
	if strings.HasPrefix(root, "/private/var/") {
		alias := strings.TrimPrefix(root, "/private")
		aliasPolicy := permissions.Policy{WriteRoots: []string{filepath.Join(alias, "project with spaces")}}
		if output := run(aliasPolicy, false, "/bin/sh", "-c", "touch alias"); output.ExitCode != 0 {
			t.Fatalf("system alias: %s", output.Output)
		}
	}
	err = os.Symlink(workspace, filepath.Join(root, "linked"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = prepareSandbox(permissions.Policy{WriteRoots: []string{filepath.Join(root, "linked")}}, workspace, []string{"/usr/bin/true"})
	if err == nil {
		t.Fatal("nested symlink writable root accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = m.AgentExec(ctx, policy, machine.ProcessRequest{OwnerID: "cancel-test", Dir: workspace, Argv: []string{"/bin/sleep", "30"}, Wait: time.Second})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("cancellation did not stop sandbox: %v", err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socketDir, err := os.MkdirTemp("/tmp", "harness-sandbox-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	for _, network := range []string{"tcp", "unix"} {
		address := "127.0.0.1:0"
		if network == "unix" {
			address = filepath.Join(socketDir, "probe.sock")
		}
		listener, err := net.Listen(network, address)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		for _, enabled := range []bool{false, true, false} {
			probePolicy := policy
			probePolicy.Network = enabled
			output := run(probePolicy, false, executable, "--sandbox-probe", network, listener.Addr().String())
			if (output.ExitCode == 0) != enabled {
				t.Fatalf("%s network=%v exit=%d output=%s", network, enabled, output.ExitCode, output.Output)
			}
		}
	}
}
