package localenv

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"harness/workspace/process"
)

func TestLocalProcessTalksUntilStdinCloses(t *testing.T) {
	spawner, err := newSpawner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer spawner.Close()

	handle, err := spawner.Spawn(context.Background(), process.Command{Program: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = handle.Stdin().Write([]byte("你好\n"))
	if err != nil {
		t.Fatal(err)
	}
	err = handle.Stdin().Close()
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(handle.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	exit, err := handle.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if exit.Code != 0 || string(output) != "你好\n" {
		t.Fatalf("进程回话不对：exit=%d output=%q", exit.Code, output)
	}

	again, err := handle.Wait()
	if err != nil || again != exit {
		t.Fatalf("Wait 应该可重复读同一个结果：exit=%+v err=%v", again, err)
	}
}

func TestLocalProcessReportsNonZeroExit(t *testing.T) {
	spawner, err := newSpawner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer spawner.Close()

	handle, err := spawner.Spawn(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "exit 7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	exit, err := handle.Wait()
	if err == nil || exit.Code != 7 {
		t.Fatalf("应看到退出码 7：exit=%d err=%v", exit.Code, err)
	}
}

func TestLocalProcessReceivesSignal(t *testing.T) {
	spawner, err := newSpawner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer spawner.Close()

	handle, err := spawner.Spawn(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "sleep 30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = handle.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := handle.Wait()
	if err == nil || exit.Code != -1 {
		t.Fatalf("收到 SIGTERM 后应被信号结束：exit=%d err=%v", exit.Code, err)
	}
}

func TestLocalProcessStopsWhenContextCancels(t *testing.T) {
	spawner, err := newSpawner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer spawner.Close()
	ctx, cancel := context.WithCancel(context.Background())

	handle, err := spawner.Spawn(ctx, process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "while :; do sleep 1; done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		_, waitErr := handle.Wait()
		done <- waitErr
	}()
	select {
	case waitErr := <-done:
		if !errors.Is(waitErr, context.Canceled) {
			t.Fatalf("应返回 context.Canceled：got %v", waitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后进程没有退出")
	}
}

func TestLocalSpawnerCloseKillsChildProcess(t *testing.T) {
	spawner, err := newSpawner(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := spawner.Spawn(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "sleep 30 & echo $!; wait"},
	})
	if err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(handle.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("子进程 PID 不对：%q", line)
	}

	err = spawner.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = handle.Wait()
	assertProcessGone(t, childPID)

	_, err = spawner.Spawn(context.Background(), process.Command{Program: "true"})
	if err == nil {
		t.Fatal("关闭后不能再启动进程")
	}
	err = spawner.Close()
	if err != nil {
		t.Fatalf("重复 Close 应该成功：%v", err)
	}
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("子进程 %d 在 Spawner.Close 后仍存在", pid)
}
