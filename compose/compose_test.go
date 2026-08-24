package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/tools"
)

func TestOpenWritesPrivateTemplateThenStops(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := t.TempDir()
	_, err := Open(home, workspace)
	if err == nil {
		t.Fatal("首次没有配置应写模板后停止")
	}
	if !strings.Contains(err.Error(), "providers.deepseek.api_key") {
		t.Fatalf("首次提示不清楚：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 1,\n  \"providers\": {\n    \"deepseek\": {\n      \"api_key\": \"\",\n      \"base_url\": \"https://api.deepseek.com\"\n    }\n  }\n}\n"
	if string(data) != want {
		t.Fatalf("配置模板不精确：%s", data)
	}
	assertPrivate(t, home, 0o700)
	assertPrivate(t, filepath.Join(home, "config.json"), 0o600)
}

func TestOpenBuildsWithExplicitWorkspace(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := t.TempDir()
	_, err := Open(home, workspace)
	if err == nil {
		t.Fatal("首次应停在模板")
	}
	config := "{\"version\":1,\"providers\":{\"deepseek\":{\"api_key\":\"test-key\",\"base_url\":\"http://127.0.0.1\"}}}"
	err = os.WriteFile(filepath.Join(home, "config.json"), []byte(config), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := Open(home, workspace)
	if err != nil {
		t.Fatalf("组装失败：%v", err)
	}
	t.Cleanup(runtime.App.Close)
	if runtime.Workspace != workspace {
		t.Fatalf("工作目录不对：%q", runtime.Workspace)
	}
	if runtime.Home != home {
		t.Fatalf("用户目录不对：%q", runtime.Home)
	}
	registry, err := tools.Get(runtime.App)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := registry.Lookup("write_file", ""); !exists {
		t.Fatal("完整组装应带 write_file")
	}
	assertPrivate(t, filepath.Join(home, "agents"), 0o700)
	assertPrivate(t, filepath.Join(home, "sessions"), 0o700)
}

func TestOpenRejectsBadConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := t.TempDir()
	_, err := Open(home, workspace)
	if err == nil {
		t.Fatal("首次应停在模板")
	}
	path := filepath.Join(home, "config.json")
	err = os.WriteFile(path, []byte(`{"version":2,"providers":{"deepseek":{"api_key":"test-key"}}}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(home, workspace)
	if err == nil || !strings.Contains(err.Error(), "版本") {
		t.Fatalf("错误版本应拒绝：%v", err)
	}
	err = os.WriteFile(path, []byte(`{"version":1,"providers":{"deepseek":{"api_key":"test-key"}},"extra":true}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(home, workspace)
	if err == nil || !strings.Contains(err.Error(), "配置不是合法 JSON") {
		t.Fatalf("未知字段应拒绝：%v", err)
	}
}

func assertPrivate(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s 权限是 %o，想要 %o", path, info.Mode().Perm(), want)
	}
}
