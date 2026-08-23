package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harness/core"
	"harness/localenv"
	"harness/tools"
)

func TestPluginRegistersWriteFile(t *testing.T) {
	root := t.TempDir()
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(
		localenv.Plugin{Root: root},
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

	output, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"notes/hello.txt","content":"你好"}`))
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

func TestPluginRequiresFiles(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(tools.Plugin{}, Plugin{})
	if err == nil {
		t.Fatal("没有 files 能力时应启动失败")
	}
}
