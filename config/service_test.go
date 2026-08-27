package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/core"
)

func TestSettingsRegisterGetAndUnregister(t *testing.T) {
	cabinet := newSettings("", map[string]map[string]string{
		"deepseek": {"base_url": "https://example.test"},
	})
	unregister, err := cabinet.Register(Drawer{
		Name:     "deepseek",
		Defaults: map[string]string{"base_url": "https://api.deepseek.com", "timeout": "30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := cabinet.Get("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if values["base_url"] != "https://example.test" || values["timeout"] != "30" {
		t.Fatalf("应盖上文件值并保留默认值：%v", values)
	}
	_, err = cabinet.Register(Drawer{Name: "deepseek", Defaults: map[string]string{}})
	if err == nil {
		t.Fatal("同名抽屉应拒绝")
	}
	unregister()
	unregister()
	_, err = cabinet.Get("deepseek")
	if err == nil {
		t.Fatal("撤销后不能再读")
	}
}

func TestSettingsEmptyFileValueKeepsDefault(t *testing.T) {
	cabinet := newSettings("", map[string]map[string]string{
		"deepseek": {"base_url": ""},
	})
	_, err := cabinet.Register(Drawer{
		Name:     "deepseek",
		Defaults: map[string]string{"base_url": "https://api.deepseek.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := cabinet.Get("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if values["base_url"] != "https://api.deepseek.com" {
		t.Fatalf("空文件值应退回默认：%v", values)
	}
}

func TestSettingsSetValidationFailureDoesNotChangeMemory(t *testing.T) {
	cabinet := newSettings("", map[string]map[string]string{
		"deepseek": {"base_url": "https://old.example"},
	})
	_, err := cabinet.Register(Drawer{
		Name:     "deepseek",
		Defaults: map[string]string{"base_url": "https://default.example"},
		Validate: func(values map[string]string) error {
			if values["base_url"] == "https://bad.example" {
				return fmt.Errorf("网址不可用")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = cabinet.Set("deepseek", map[string]string{"base_url": "https://bad.example"})
	if err == nil {
		t.Fatal("非法配置应被拒绝")
	}
	values, err := cabinet.Get("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if values["base_url"] != "https://old.example" {
		t.Fatalf("写入失败后不应改变内存：%v", values)
	}
}

func TestCredentialsResolveRequiresValue(t *testing.T) {
	vault := newCredentials("", map[string]map[string]string{
		"deepseek": {"api_key": "sk-secret"},
	})
	_, err := vault.Resolve("deepseek", "api_key")
	if err == nil {
		t.Fatal("没登记就不能领")
	}
	unregister, err := vault.Register(Need{Drawer: "deepseek", Key: "api_key"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := vault.Resolve("deepseek", "api_key")
	if err != nil || value != "sk-secret" {
		t.Fatalf("应领到明文：%q err=%v", value, err)
	}
	if !vault.Configured("deepseek", "api_key") {
		t.Fatal("应报告已配置")
	}
	unregister()
	_, err = vault.Resolve("deepseek", "api_key")
	if err == nil || err.Error() != "缺少钥匙" {
		t.Fatalf("撤销后应缺少钥匙：%v", err)
	}
}

func TestCredentialsEmptyIsMissingAndDoesNotLeak(t *testing.T) {
	vault := newCredentials("", map[string]map[string]string{
		"deepseek": {"api_key": ""},
	})
	_, err := vault.Register(Need{Drawer: "deepseek", Key: "api_key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = vault.Resolve("deepseek", "api_key")
	if err == nil || err.Error() != "缺少钥匙" {
		t.Fatalf("空钥匙应报缺：%v", err)
	}
	if strings.Contains(err.Error(), "sk-") {
		t.Fatalf("错误里不能出现密钥：%v", err)
	}
	if vault.Configured("deepseek", "api_key") {
		t.Fatal("空钥匙不算已配置")
	}
}

func TestCredentialsSetFailureDoesNotChangeMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "credentials.yaml")
	vault := newCredentials(path, map[string]map[string]string{
		"deepseek": {"api_key": "old-secret"},
	})
	_, err := vault.Register(Need{Drawer: "deepseek", Key: "api_key"})
	if err != nil {
		t.Fatal(err)
	}
	err = vault.Set("deepseek", "api_key", "new-secret")
	if err == nil {
		t.Fatal("写入不存在目录时应失败")
	}
	value, err := vault.Resolve("deepseek", "api_key")
	if err != nil {
		t.Fatal(err)
	}
	if value != "old-secret" {
		t.Fatalf("写入失败后不应改变内存：%q", value)
	}
}

func TestPluginLoadsFiles(t *testing.T) {
	home := t.TempDir()
	err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("version: 2\nsettings:\n  deepseek:\n    base_url: https://example.test\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(home, "credentials.yaml"), []byte("deepseek:\n  api_key: sk-file\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	app := core.New()
	t.Cleanup(app.Close)
	err = app.Install(Plugin{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := GetSettings(app)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := GetCredentials(app)
	if err != nil {
		t.Fatal(err)
	}
	_, err = settings.Register(Drawer{Name: "deepseek", Defaults: map[string]string{"base_url": "https://api.deepseek.com"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = creds.Register(Need{Drawer: "deepseek", Key: "api_key"})
	if err != nil {
		t.Fatal(err)
	}
	values, err := settings.Get("deepseek")
	if err != nil || values["base_url"] != "https://example.test" {
		t.Fatalf("应读到文件里的网址：%v err=%v", values, err)
	}
	value, err := creds.Resolve("deepseek", "api_key")
	if err != nil || value != "sk-file" {
		t.Fatalf("应读到文件里的钥匙：%q err=%v", value, err)
	}
}

func TestPluginTightensExistingCredentialsPermissions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "credentials.yaml")
	err := os.WriteFile(path, []byte("deepseek:\n  api_key: sk-file\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	app := core.New()
	t.Cleanup(app.Close)
	err = app.Install(Plugin{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("已有钥匙文件权限是 %o", info.Mode().Perm())
	}
}

func TestPluginCreatesEmptyCredentialsFile(t *testing.T) {
	home := t.TempDir()
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(Plugin{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, "credentials.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("钥匙文件权限是 %o", info.Mode().Perm())
	}
}
