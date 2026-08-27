package config

import (
	"fmt"
	"os"
	"path/filepath"

	"harness/core"
)

// Plugin 把抽屉柜和保险柜装进场地（能力名 "settings"、"credentials"）。
type Plugin struct {
	Home string // 用户私有目录，里面是 config.yaml 和 credentials.yaml
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "config"
}

// Start 读取两份文件，再登记两个服务。
func (p Plugin) Start(app *core.App) error {
	if p.Home == "" {
		return fmt.Errorf("config 缺少用户目录")
	}
	configPath := filepath.Join(p.Home, "config.yaml")
	credentialsPath := filepath.Join(p.Home, "credentials.yaml")
	user, err := readSettings(configPath)
	if err != nil {
		return err
	}
	secrets, err := readCredentials(credentialsPath)
	if err != nil {
		return err
	}
	app.RegisterService("settings", newSettings(configPath, user))
	app.RegisterService("credentials", newCredentials(credentialsPath, secrets))
	return nil
}

// InstallMemory 把内存抽屉柜和保险柜装进场地，给单测用。
func InstallMemory(app *core.App, user map[string]map[string]string, secrets map[string]map[string]string) {
	app.RegisterService("settings", newSettings("", cloneTable(user)))
	app.RegisterService("credentials", newCredentials("", cloneTable(secrets)))
}

func mustMkdir(path string) error {
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	return nil
}
