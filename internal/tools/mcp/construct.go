package mcp

import (
	"context"
	"errors"
	"fmt"
	"harness/internal/approvals"
	"harness/internal/persist"
	"os"
	"time"
)

const startupTimeout = 30 * time.Second

// New 读取用户配置并建立 MCP 来源；调用方负责 Close。
func New(files *persist.Files, approvalService *approvals.Service) (*Provider, error) {
	config := configFile{}
	data, err := files.Read("mcp.json")
	if err == nil {
		config, err = parseConfig(data, "mcp.json")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	launchDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("tools-mcp: get launch directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	provider, err := newProvider(ctx, config, launchDir)
	if err != nil {
		return nil, err
	}
	provider.approvals = approvalService
	return provider, nil
}
