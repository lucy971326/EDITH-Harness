// Package backend 显式组装进程中的领域服务和 appserver。
package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"harness/internal/agents"
	"harness/internal/approvals"
	"harness/internal/appserver"
	"harness/internal/commands"
	compactcmd "harness/internal/commands/compact"
	"harness/internal/conversations"
	"harness/internal/events"
	"harness/internal/hooks"
	"harness/internal/llm"
	"harness/internal/loops"
	"harness/internal/loops/react"
	machinelocal "harness/internal/machine/local"
	"harness/internal/persist"
	"harness/internal/runner"
	"harness/internal/session"
	"harness/internal/session/settings"
	"harness/internal/skills"
	skillsbuiltin "harness/internal/skills/builtin"
	skillsfilesystem "harness/internal/skills/filesystem"
	"harness/internal/subagents"
	"harness/internal/tools"
	applypatchtool "harness/internal/tools/applypatch"
	exectool "harness/internal/tools/exec"
	mcptool "harness/internal/tools/mcp"
	subagenttools "harness/internal/tools/subagents"
)

// 活对象。Backend 拥有一个进程的服务和逆序关闭动作。
type Backend struct {
	Server *appserver.Server
	close  func() error
}

func UserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".harness"), nil
}

// Open 先独占数据目录，再按使用顺序组装服务；失败时释放已取得的资源。
func Open(dataDir string) (backend *Backend, result error) {
	lock, err := persist.LockRoot(dataDir)
	if err != nil {
		return nil, err
	}
	closers := []func() error{lock.Close}
	defer func() {
		if result != nil {
			result = errors.Join(result, closeReverse(closers))
		}
	}()

	files, err := persist.NewFiles(dataDir)
	if err != nil {
		return nil, err
	}
	disk := session.NewPersistence(files)
	settingsStore := settings.NewStore(files)
	sessions := session.NewStore(disk)
	models, err := llm.New(files)
	if err != nil {
		return nil, err
	}
	machineService, err := machinelocal.New()
	if err != nil {
		return nil, err
	}
	closers = append(closers, machineService.Close)
	toolRegistry := tools.NewRegistry()
	hookService, err := hooks.NewService(files, machineService)
	if err != nil {
		return nil, err
	}
	toolRegistry.SetPreToolUse(hookService)
	approvalService, err := approvals.Open(files, models, sessions)
	if err != nil {
		return nil, err
	}
	closers = append(closers, approvalService.Close)
	err = toolRegistry.Register(applypatchtool.New(machineService, machineService, approvalService))
	if err != nil {
		return nil, err
	}
	err = exectool.Register(toolRegistry, machineService, machineService, approvalService)
	if err != nil {
		return nil, err
	}
	mcpProvider, err := mcptool.New(files, approvalService)
	if err != nil {
		return nil, err
	}
	closers = append(closers, mcpProvider.Close)
	err = toolRegistry.RegisterProvider(mcpProvider)
	if err != nil {
		return nil, err
	}
	registry := events.NewRegistry()
	loopRegistry := loops.NewRegistry()
	err = loopRegistry.Register(react.New(models, toolRegistry))
	if err != nil {
		return nil, err
	}
	skillService := skills.NewRegistry()
	builtinSkills, err := skillsbuiltin.New(files)
	if err != nil {
		return nil, err
	}
	err = skillService.Register(builtinSkills)
	if err != nil {
		return nil, err
	}
	err = skillService.Register(skillsfilesystem.New(machineService, files))
	if err != nil {
		return nil, err
	}
	agentService, err := agents.NewService(agents.NewStore(files), settingsStore, loopRegistry, toolRegistry, skillService)
	if err != nil {
		return nil, err
	}
	commandService := commands.NewRegistry()
	runService, err := runner.NewRunner(sessions, settingsStore, agentService, loopRegistry, registry, models, toolRegistry, files, machineService)
	if err != nil {
		return nil, err
	}
	closers = append(closers, func() error { runService.Close(); return nil })
	subagentFiles, err := files.Scope("subagents")
	if err != nil {
		return nil, err
	}
	subagentService, err := subagents.NewSubagents(sessions, settingsStore, agentService, models, runService, registry, subagentFiles)
	if err != nil {
		return nil, err
	}
	closers = append(closers, subagentService.Close)
	err = subagenttools.Register(toolRegistry, subagentService)
	if err != nil {
		return nil, err
	}
	conversationService, err := conversations.New(sessions, settingsStore, agentService, models, runService, commandService, subagentService, approvalService)
	if err != nil {
		return nil, err
	}
	err = commandService.Register(compactcmd.New(runService))
	if err != nil {
		return nil, err
	}

	// 接入最后创建、最先关闭；组装成功之前不开放监听。
	server := appserver.New()
	closers = append(closers, server.Close)
	err = server.BindHarness(conversationService, runService, registry)
	if err != nil {
		return nil, err
	}
	err = server.BindApprovals(approvalService)
	if err != nil {
		return nil, err
	}
	err = server.BindHooks(hookService)
	if err != nil {
		return nil, err
	}
	err = server.BindFilesystem(machineService)
	if err != nil {
		return nil, err
	}
	err = server.BindCommandExec(machineService)
	if err != nil {
		return nil, err
	}
	err = server.BindModels(models)
	if err != nil {
		return nil, err
	}
	err = server.BindAgents(agentService)
	if err != nil {
		return nil, err
	}
	err = server.BindSkills(skillService)
	if err != nil {
		return nil, err
	}
	err = server.BindCommands(commandService)
	if err != nil {
		return nil, err
	}
	return &Backend{Server: server, close: func() error { return closeReverse(closers) }}, nil
}

func (b *Backend) Close() error {
	return b.close()
}

func closeReverse(closers []func() error) (result error) {
	for index := len(closers) - 1; index >= 0; index-- {
		result = errors.Join(result, closers[index]())
	}
	return result
}
