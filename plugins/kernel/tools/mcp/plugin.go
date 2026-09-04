// Package mcp 把 MCP Server 提供的工具填入 Harness tools 登记处。
package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"harness/kernel/host"
	"harness/kernel/tools"
)

const startupTimeout = 30 * time.Second

// 活对象。持有 MCP Provider 及其打开的全部连接。
type Plugin struct {
	userConfigPath string
	provider       *Provider
}

// New 造一个读取指定用户配置的 MCP 插件。
func New(userConfigPath string) *Plugin {
	return &Plugin{userConfigPath: userConfigPath}
}

func (p *Plugin) Name() string { return "tools-mcp" }

func (p *Plugin) Start(h *host.Host) error {
	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return fmt.Errorf("tools-mcp: resolve tools: %w", err)
	}
	launchDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("tools-mcp: get launch directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	provider, err := newProvider(ctx, p.userConfigPath, launchDir)
	if err != nil {
		return err
	}
	err = registry.RegisterProvider(provider)
	if err != nil {
		_ = provider.Close()
		return err
	}
	p.provider = provider
	return nil
}

func (p *Plugin) Close() error {
	if p.provider == nil {
		return nil
	}
	provider := p.provider
	p.provider = nil
	return provider.Close()
}
