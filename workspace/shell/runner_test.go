package shell_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"harness/core"
	"harness/localenv"
	"harness/workspace/process"
	"harness/workspace/shell"
)

func newTestRunner(t *testing.T, limit int) (shell.Runner, *core.App) {
	t.Helper()
	app := core.New()
	err := app.Install(localenv.Plugin{Root: t.TempDir(), OutputLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := shell.Get(app)
	if err != nil {
		app.Close()
		t.Fatal(err)
	}
	return runner, app
}

func TestRunnerReturnsOutputAndExitCode(t *testing.T) {
	runner, app := newTestRunner(t, 1024)
	defer app.Close()

	result, err := runner.Run(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "printf hello; printf warning >&2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "hello" || result.Stderr != "warning" || result.ExitCode != 0 || result.Truncated {
		t.Fatalf("命令结果不对：%+v", result)
	}
}

func TestRunnerKeepsOutputWhenCommandFails(t *testing.T) {
	runner, app := newTestRunner(t, 1024)
	defer app.Close()

	result, err := runner.Run(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "printf before; exit 9"},
	})
	if err == nil {
		t.Fatal("非零退出应返 error")
	}
	if result.Stdout != "before" || result.ExitCode != 9 {
		t.Fatalf("失败命令也应保留输出和退出码：%+v", result)
	}
}

func TestRunnerTruncatesWithoutBlockingCommand(t *testing.T) {
	runner, app := newTestRunner(t, 4)
	defer app.Close()

	result, err := runner.Run(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "yes abcdef | head -c 200000; printf 123456 >&2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "abcd" || result.Stderr != "1234" || !result.Truncated {
		t.Fatalf("输出应各自保留前 4 字节：%+v", result)
	}
}

func TestRunnerDoesNotReturnHalfAChineseCharacter(t *testing.T) {
	runner, app := newTestRunner(t, 5)
	defer app.Close()

	result, err := runner.Run(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "printf '你好'"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "你" || !result.Truncated {
		t.Fatalf("不能把半个汉字交出去：%q", result.Stdout)
	}
}

func TestRunnerStopsAtContextDeadline(t *testing.T) {
	runner, app := newTestRunner(t, 1024)
	defer app.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "sleep 30"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时应返 context.DeadlineExceeded：got %v", err)
	}
}
