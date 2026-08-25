package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harness/core"
	"harness/environment"
	"harness/localenv"
	"harness/tools"
)

func TestPluginRegistersWriteFile(t *testing.T) {
	root := t.TempDir()
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(
		localenv.Plugin{},
		tools.Plugin{},
		Plugin{},
	)
	if err != nil {
		t.Fatalf("装插件失败：%v", err)
	}

	registry, err := tools.Get(app)
	if err != nil {
		t.Fatalf("取 tools 失败：%v", err)
	}
	tool, exists := registry.Lookup("write_file", "")
	if !exists {
		t.Fatal("write_file 应登记进 tools")
	}
	provider, err := environment.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	scope := app.ForChild("测试会话")
	t.Cleanup(scope.Close)
	err = provider.Mount(scope, root)
	if err != nil {
		t.Fatal(err)
	}

	output, err := tool.Execute(context.Background(), scope, json.RawMessage(`{"path":"notes/hello.txt","content":"你好"}`))
	if err != nil {
		t.Fatalf("调用 write_file 失败：%v", err)
	}
	if output != "已写入文件：notes/hello.txt" {
		t.Fatalf("返回文字不对：%q", output)
	}

	data, err := os.ReadFile(filepath.Join(root, "notes", "hello.txt"))
	if err != nil {
		t.Fatalf("读回文件失败：%v", err)
	}
	if string(data) != "你好" {
		t.Fatalf("写入内容不对：%q", data)
	}
}

func TestWriteFileUsesEachSessionEnvironment(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(localenv.Plugin{}, tools.Plugin{}, Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := environment.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	firstScope := app.ForChild("会话一")
	secondScope := app.ForChild("会话二")
	t.Cleanup(firstScope.Close)
	t.Cleanup(secondScope.Close)
	err = provider.Mount(firstScope, firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	err = provider.Mount(secondScope, secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	tool, exists := registry.Lookup("write_file", "")
	if !exists {
		t.Fatal("write_file 没登记")
	}
	_, err = tool.Execute(context.Background(), firstScope, json.RawMessage(`{"path":"owner.txt","content":"项目一"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(context.Background(), secondScope, json.RawMessage(`{"path":"owner.txt","content":"项目二"}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(firstRoot, "owner.txt"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(secondRoot, "owner.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "项目一" || string(second) != "项目二" {
		t.Fatalf("两个会话写串目录：%q / %q", first, second)
	}
}

func TestPluginRequiresTools(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(Plugin{})
	if err == nil {
		t.Fatal("没有 tools 能力时应启动失败")
	}
}
