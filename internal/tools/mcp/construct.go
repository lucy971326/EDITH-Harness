package mcp

import (
	"fmt"
	"harness/internal/approvals"
	"harness/internal/persist"
	"os"
	"time"
)

const startupTimeout = 30 * time.Second

// New 建立 MCP 来源；连接在新 Run 准备或用户显式重试时创建，调用方负责 Close。
func New(files *persist.Files, approvalService *approvals.Service) (*Provider, error) {
	launchDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("tools-mcp: get launch directory: %w", err)
	}
	authFiles, err := files.Scope("mcp", "oauth")
	if err != nil {
		return nil, fmt.Errorf("tools-mcp: open OAuth credential files: %w", err)
	}
	authStore := newOAuthStore()
	authStore.files = authFiles
	provider := &Provider{files: files, launchDir: launchDir, approvals: approvalService,
		current: make(map[string]*workspaceState), runs: make(map[string]*workspaceState), authStore: authStore, authTasks: make(map[string]*oauthTask)}
	return provider, nil
}
