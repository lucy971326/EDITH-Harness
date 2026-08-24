package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"harness/tools"
	"harness/ui"
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
	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != configTemplate {
		t.Fatalf("配置模板不精确：%s", data)
	}
	assertPrivate(t, home, 0o700)
	assertPrivate(t, path, 0o600)
}

func TestOpenBuildsWithExplicitWorkspace(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := t.TempDir()
	writeConfig(t, home, validConfig())

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
	_, err = ui.Get(runtime.App)
	if err != nil {
		t.Fatalf("完整组装应带终端 UI：%v", err)
	}
	assertPrivate(t, filepath.Join(home, "agents"), 0o700)
	assertPrivate(t, filepath.Join(home, "sessions"), 0o700)
}

func TestOpenAllowsNoToolPlugins(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := t.TempDir()
	config := strings.Replace(validConfig(), "tool_plugins:\n    - files", "tool_plugins: []", 1)
	writeConfig(t, home, config)

	runtime, err := Open(home, workspace)
	if err != nil {
		t.Fatalf("空工具列表也应能组装：%v", err)
	}
	t.Cleanup(runtime.App.Close)
	registry, err := tools.Get(runtime.App)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := registry.Lookup("write_file", ""); exists {
		t.Fatal("没有选择 files 插件时不应登记 write_file")
	}
}

func TestDumpConfigRoundTrips(t *testing.T) {
	firstHome := filepath.Join(t.TempDir(), "first")
	secondHome := filepath.Join(t.TempDir(), "second")
	writeConfig(t, firstHome, validConfig())

	var dumped bytes.Buffer
	err := DumpConfig(firstHome, &dumped)
	if err != nil {
		t.Fatalf("导出配置失败：%v", err)
	}
	if !strings.Contains(dumped.String(), "api_key: test-key") {
		t.Fatalf("导出配置应保留可重新组装所需的密钥：%s", dumped.String())
	}
	writeConfig(t, secondHome, dumped.String())

	firstConfig, err := loadConfig(firstHome)
	if err != nil {
		t.Fatal(err)
	}
	secondConfig, err := loadConfig(secondHome)
	if err != nil {
		t.Fatal(err)
	}
	firstSelection, err := selectPlugins(firstConfig, firstHome, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	secondSelection, err := selectPlugins(secondConfig, secondHome, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	var firstNames []string
	for _, plugin := range firstSelection.ordered() {
		firstNames = append(firstNames, plugin.Name())
	}
	var secondNames []string
	for _, plugin := range secondSelection.ordered() {
		secondNames = append(secondNames, plugin.Name())
	}
	if !slices.Equal(firstNames, secondNames) {
		t.Fatalf("导出再导入后的安装顺序不同：%v 和 %v", firstNames, secondNames)
	}
}

func TestOpenRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		change func(string) string
		want   string
	}{
		{
			name: "错误版本",
			change: func(config string) string {
				return strings.Replace(config, "version: 1", "version: 2", 1)
			},
			want: "版本必须是 1",
		},
		{
			name: "未知字段",
			change: func(config string) string {
				return config + "extra: true\n"
			},
			want: "配置不是合法 YAML",
		},
		{
			name: "未知档案存储",
			change: func(config string) string {
				return strings.Replace(config, "profile_store: profilejson", "profile_store: sqlite", 1)
			},
			want: "未知的 plugins.profile_store：sqlite",
		},
		{
			name: "未知账本",
			change: func(config string) string {
				return strings.Replace(config, "journal: jsonl", "journal: sqlite", 1)
			},
			want: "未知的 plugins.journal：sqlite",
		},
		{
			name: "未知模型适配器",
			change: func(config string) string {
				return strings.Replace(config, "- deepseek", "- openai", 1)
			},
			want: "未知的 plugins.llm_adapters：openai",
		},
		{
			name: "未知执行环境",
			change: func(config string) string {
				return strings.Replace(config, "environment: localenv", "environment: sandbox", 1)
			},
			want: "未知的 plugins.environment：sandbox",
		},
		{
			name: "未知工具插件",
			change: func(config string) string {
				return strings.Replace(config, "- files", "- bash", 1)
			},
			want: "未知的 plugins.tool_plugins：bash",
		},
		{
			name: "未知 Runner",
			change: func(config string) string {
				return strings.Replace(config, "runner: loop", "runner: debate", 1)
			},
			want: "未知的 plugins.runner：debate",
		},
		{
			name: "未知 UI",
			change: func(config string) string {
				return strings.Replace(config, "ui: terminal", "ui: web", 1)
			},
			want: "未知的 plugins.ui：web",
		},
		{
			name: "重复模型适配器",
			change: func(config string) string {
				return strings.Replace(config, "    - deepseek", "    - deepseek\n    - deepseek", 1)
			},
			want: "plugins.llm_adapters 重复选择了 deepseek",
		},
		{
			name: "重复工具插件",
			change: func(config string) string {
				return strings.Replace(config, "    - files", "    - files\n    - files", 1)
			},
			want: "plugins.tool_plugins 重复选择了 files",
		},
		{
			name: "缺少模型适配器",
			change: func(config string) string {
				return strings.Replace(config, "  llm_adapters:\n    - deepseek\n", "", 1)
			},
			want: "配置至少需要一个 plugins.llm_adapters",
		},
		{
			name: "缺少工具列表",
			change: func(config string) string {
				return strings.Replace(config, "  tool_plugins:\n    - files\n", "", 1)
			},
			want: "配置缺少 plugins.tool_plugins",
		},
		{
			name: "多份 YAML 文档",
			change: func(config string) string {
				return config + "---\nversion: 1\n"
			},
			want: "配置只能有一份 YAML 文档",
		},
		{
			name: "缺少所选模型密钥",
			change: func(config string) string {
				return strings.Replace(config, `api_key: "test-key"`, `api_key: ""`, 1)
			},
			want: "配置缺少 providers.deepseek.api_key",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			workspace := t.TempDir()
			writeConfig(t, home, test.change(validConfig()))

			_, err := Open(home, workspace)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("错误应包含 %q，实际是 %v", test.want, err)
			}
		})
	}
}

func TestSelectedPluginsKeepFixedOrder(t *testing.T) {
	config := Config{
		Version: 1,
		Plugins: PluginConfig{
			ProfileStore: "profilejson",
			Journal:      "jsonl",
			LLMAdapters:  []string{"deepseek"},
			Environment:  "localenv",
			ToolPlugins:  []string{"files"},
			Runner:       "loop",
			UI:           "terminal",
		},
		Providers: ProviderConfigs{DeepSeek: DeepSeekConfig{
			APIKey: "test-key",
		}},
	}
	selected, err := selectPlugins(config, "/private/home", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, plugin := range selected.ordered() {
		names = append(names, plugin.Name())
	}
	want := []string{
		"persistence-profilejson",
		"persistence-jsonl",
		"session",
		"llm",
		"llm-deepseek",
		"tools",
		"localenv",
		"tool-files",
		"agents",
		"loop",
		"ui-terminal",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("安装顺序是 %v，想要 %v", names, want)
	}
}

func validConfig() string {
	return strings.Replace(configTemplate, `api_key: ""`, `api_key: "test-key"`, 1)
}

func writeConfig(t *testing.T, home string, config string) {
	t.Helper()
	err := os.MkdirAll(home, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(home, "config.yaml"), []byte(config), 0o600)
	if err != nil {
		t.Fatal(err)
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
