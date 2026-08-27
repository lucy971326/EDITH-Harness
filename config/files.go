package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

type settingsFile struct {
	Settings map[string]map[string]string `yaml:"settings"`
}

func readSettings(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读配置失败：%w", err)
	}
	var file settingsFile
	err = yaml.Unmarshal(data, &file)
	if err != nil {
		return nil, fmt.Errorf("配置不是合法 YAML：%w", err)
	}
	return cloneTable(file.Settings), nil
}

func readCredentials(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		err = writeEmptyCredentials(path)
		if err != nil {
			return nil, err
		}
		return map[string]map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读钥匙失败：%w", err)
	}
	err = os.Chmod(path, 0o600)
	if err != nil {
		return nil, fmt.Errorf("收紧钥匙文件权限失败：%w", err)
	}
	var secrets map[string]map[string]string
	err = yaml.Unmarshal(data, &secrets)
	if err != nil {
		return nil, fmt.Errorf("钥匙文件不是合法 YAML：%w", err)
	}
	return cloneTable(secrets), nil
}

func writeEmptyCredentials(path string) error {
	err := mustMkdir(path)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, []byte("{}\n"), 0o600)
	if err != nil {
		return fmt.Errorf("写钥匙模板失败：%w", err)
	}
	return nil
}
