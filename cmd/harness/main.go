package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	webclient "harness/clients/web"
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

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	handled, workerErr := machinelocal.RunFileWorker(os.Args[1:], os.Stdin, os.Stdout)
	if handled {
		return workerErr
	}

	dataDir, err := userDataDir()
	if err != nil {
		return err
	}

	// 依赖按使用顺序构造；每取得一份资源便安排逆序收尾。
	files, err := persist.NewFiles(dataDir)
	if err != nil {
		return err
	}
	disk := session.NewPersistence(files)
	settingsStore := settings.NewStore(files)
	sessions := session.NewStore(disk)
	models, err := llm.New(files)
	if err != nil {
		return err
	}
	machineService, err := machinelocal.New()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, machineService.Close()) }()
	toolRegistry := tools.NewRegistry()
	hookService, err := hooks.NewService(files, machineService)
	if err != nil {
		return err
	}
	toolRegistry.SetPreToolUse(hookService)
	approvalService, err := approvals.Open(files, models, sessions)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, approvalService.Close()) }()
	err = toolRegistry.Register(applypatchtool.New(machineService, machineService, approvalService))
	if err != nil {
		return err
	}
	err = exectool.Register(toolRegistry, machineService, machineService, approvalService)
	if err != nil {
		return err
	}
	mcpProvider, err := mcptool.New(files, approvalService)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, mcpProvider.Close()) }()
	err = toolRegistry.RegisterProvider(mcpProvider)
	if err != nil {
		return err
	}
	registry := events.NewRegistry()
	loopRegistry := loops.NewRegistry()
	err = loopRegistry.Register(react.New(models, toolRegistry))
	if err != nil {
		return err
	}
	skillService := skills.NewRegistry()
	builtinSkills, err := skillsbuiltin.New(files)
	if err != nil {
		return err
	}
	err = skillService.Register(builtinSkills)
	if err != nil {
		return err
	}
	err = skillService.Register(skillsfilesystem.New(machineService, files))
	if err != nil {
		return err
	}
	agentService, err := agents.NewService(agents.NewStore(files), settingsStore, loopRegistry, toolRegistry, skillService)
	if err != nil {
		return err
	}
	commandService := commands.NewRegistry()
	runService, err := runner.NewRunner(sessions, settingsStore, agentService, loopRegistry, registry, models, toolRegistry, files, machineService)
	if err != nil {
		return err
	}
	defer runService.Close()
	subagentFiles, err := files.Scope("subagents")
	if err != nil {
		return err
	}
	subagentService, err := subagents.NewSubagents(sessions, settingsStore, agentService, models, runService, registry, subagentFiles)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, subagentService.Close()) }()
	err = subagenttools.Register(toolRegistry, subagentService)
	if err != nil {
		return err
	}
	conversationService, err := conversations.New(sessions, settingsStore, agentService, models, runService, commandService, subagentService, approvalService)
	if err != nil {
		return err
	}
	err = commandService.Register(compactcmd.New(runService))
	if err != nil {
		return err
	}

	// 接入最后创建、最先关闭；组装成功之前不开放监听。
	server := appserver.New()
	defer func() { result = errors.Join(result, server.Close()) }()
	err = server.BindHarness(conversationService, runService, registry)
	if err != nil {
		return err
	}
	err = server.BindApprovals(approvalService)
	if err != nil {
		return err
	}
	err = server.BindHooks(hookService)
	if err != nil {
		return err
	}
	err = server.BindFilesystem(machineService)
	if err != nil {
		return err
	}
	err = server.BindCommandExec(machineService)
	if err != nil {
		return err
	}
	err = server.BindModels(models)
	if err != nil {
		return err
	}
	err = server.BindAgents(agentService)
	if err != nil {
		return err
	}
	err = server.BindSkills(skillService)
	if err != nil {
		return err
	}
	err = server.BindCommands(commandService)
	if err != nil {
		return err
	}
	webHandler, err := webclient.Handler()
	if err != nil {
		return err
	}
	url, err := server.Listen("127.0.0.1:8888", webHandler)
	if err != nil {
		return err
	}
	err = openBrowser(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法自动打开浏览器：%v\n", err)
	}
	fmt.Printf("Harness 已启动：%s\n", url)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

func userDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".harness"), nil
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		command = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported operating system %q", runtime.GOOS)
	}
	err := command.Start()
	if err != nil {
		return err
	}
	err = command.Process.Release()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
