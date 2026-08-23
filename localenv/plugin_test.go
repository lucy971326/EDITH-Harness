package localenv

import (
	"bufio"
	"context"
	"strconv"
	"strings"
	"testing"

	"harness/core"
	"harness/workspace/files"
	"harness/workspace/process"
	"harness/workspace/shell"
)

func TestPluginRegistersOneSharedEnvironment(t *testing.T) {
	app := core.New()
	defer app.Close()
	err := app.Install(Plugin{Root: t.TempDir(), OutputLimit: 1024})
	if err != nil {
		t.Fatal(err)
	}

	fileStore, err := files.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	_, err = process.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := shell.Get(app)
	if err != nil {
		t.Fatal(err)
	}

	err = fileStore.Write(context.Background(), "shared.txt", []byte("同一个院子"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), process.Command{
		Program: "/bin/sh",
		Args:    []string{"-c", "cat shared.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "同一个院子" {
		t.Fatalf("files 写的文件必须能被 shell 读到：%q", result.Stdout)
	}
}

func TestAppCloseKillsWholeProcessTree(t *testing.T) {
	app := core.New()
	err := app.Install(Plugin{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	spawner, err := process.Get(app)
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

	app.Close()
	_, _ = handle.Wait()
	assertProcessGone(t, childPID)
}

func TestPluginRejectsMissingRootWithoutRegisteringServices(t *testing.T) {
	app := core.New()
	defer app.Close()
	err := app.Install(Plugin{Root: t.TempDir() + "/missing"})
	if err == nil {
		t.Fatal("不存在的根目录应启动失败")
	}
	_, err = files.Get(app)
	if err == nil {
		t.Fatal("启动失败后不应留下 files 能力")
	}
}
