package config

import (
	"fmt"
	"os"
	"sync"

	"go.yaml.in/yaml/v3"
)

var _ Credentials = (*credentials)(nil)

type credentials struct {
	mu      sync.Mutex
	path    string
	needs   map[string]*Need
	secrets map[string]map[string]string
}

func newCredentials(path string, secrets map[string]map[string]string) *credentials {
	if secrets == nil {
		secrets = map[string]map[string]string{}
	}
	return &credentials{
		path:    path,
		needs:   map[string]*Need{},
		secrets: secrets,
	}
}

func needID(drawer string, key string) string {
	return drawer + "/" + key
}

// Register 声明一把钥匙；同名重复是组装错误。
func (c *credentials) Register(need Need) (func(), error) {
	normalized, err := normalizeNeed(need)
	if err != nil {
		return nil, err
	}
	id := needID(normalized.Drawer, normalized.Key)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, taken := c.needs[id]
	if taken {
		return nil, fmt.Errorf("钥匙 %s 已登记", id)
	}
	entry := &Need{}
	*entry = normalized
	c.needs[id] = entry
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		current, exists := c.needs[id]
		if !exists {
			return
		}
		if current != entry {
			return
		}
		delete(c.needs, id)
	}, nil
}

// Resolve 把明文交给调用方；没登记、没填或空值都说「缺少钥匙」，不带密钥片段。
func (c *credentials) Resolve(drawer string, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := needID(drawer, key)
	_, registered := c.needs[id]
	if !registered {
		return "", fmt.Errorf("缺少钥匙")
	}
	value := c.secrets[drawer][key]
	if value == "" {
		return "", fmt.Errorf("缺少钥匙")
	}
	return value, nil
}

// Configured 只回答这把已登记的钥匙现在有没有值。
func (c *credentials) Configured(drawer string, key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, registered := c.needs[needID(drawer, key)]
	if !registered {
		return false
	}
	return c.secrets[drawer][key] != ""
}

// Set 写入一把钥匙并落盘。空值等于删除。
func (c *credentials) Set(drawer string, key string, value string) error {
	_, err := normalizeNeed(Need{Drawer: drawer, Key: key})
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	candidate := cloneTable(c.secrets)
	if value == "" {
		row := candidate[drawer]
		if row != nil {
			delete(row, key)
			if len(row) == 0 {
				delete(candidate, drawer)
			}
		}
	} else {
		row := candidate[drawer]
		if row == nil {
			row = map[string]string{}
			candidate[drawer] = row
		}
		row[key] = value
	}
	err = c.persistLocked(candidate)
	if err != nil {
		return err
	}
	c.secrets = candidate
	return nil
}

func (c *credentials) persistLocked(secrets map[string]map[string]string) error {
	if c.path == "" {
		return nil
	}
	out, err := yaml.Marshal(cloneTable(secrets))
	if err != nil {
		return fmt.Errorf("写钥匙失败：%w", err)
	}
	err = os.WriteFile(c.path, out, 0o600)
	if err != nil {
		return fmt.Errorf("写钥匙失败：%w", err)
	}
	return nil
}
