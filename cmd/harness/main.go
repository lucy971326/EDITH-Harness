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

	"harness/appserver"
	webclient "harness/clients/web"
	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/skills"
	"harness/kernel/subagents"
	"harness/kernel/tools"
	compactcmd "harness/plugins/commands/compact"
	"harness/plugins/loops/react"
	machinelocal "harness/plugins/machine/local"
	skillsbuiltin "harness/plugins/skills/builtin"
	skillsfilesystem "harness/plugins/skills/filesystem"
	bashtool "harness/plugins/tools/bash"
	edittool "harness/plugins/tools/edit"
	mcptool "harness/plugins/tools/mcp"
	readtool "harness/plugins/tools/read"
	subagenttools "harness/plugins/tools/subagents"
	writetool "harness/plugins/tools/write"
	harnessproduct "harness/products/harness"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	dataDir, err := userDataDir()
	if err != nil {
		return err
	}

	h := host.NewHost()
	server := appserver.New()
	defer func() {
		// 入口先关闭 Client 接入与调用，再拆产品和执行服务。
		result = errors.Join(result, server.Close(), h.Close())
	}()
	err = h.Install(&persist.Plugin{Dir: dataDir})
	if err != nil {
		return err
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		return err
	}
	err = h.Install(&llm.Plugin{})
	if err != nil {
		return err
	}
	err = h.Install(machinelocal.New())
	if err != nil {
		return err
	}
	err = h.Install(tools.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(readtool.New())
	if err != nil {
		return err
	}
	err = h.Install(writetool.New())
	if err != nil {
		return err
	}
	err = h.Install(edittool.New())
	if err != nil {
		return err
	}
	err = h.Install(bashtool.New())
	if err != nil {
		return err
	}
	err = h.Install(mcptool.New(filepath.Join(dataDir, "mcp.json")))
	if err != nil {
		return err
	}
	err = h.Install(events.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(loops.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(react.New())
	if err != nil {
		return err
	}
	err = h.Install(skills.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(skillsbuiltin.New())
	if err != nil {
		return err
	}
	err = h.Install(skillsfilesystem.New())
	if err != nil {
		return err
	}
	err = h.Install(agents.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(commands.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(runner.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(subagents.NewPlugin(dataDir))
	if err != nil {
		return err
	}
	err = h.Install(subagenttools.New())
	if err != nil {
		return err
	}
	err = h.Install(harnessproduct.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(compactcmd.New())
	if err != nil {
		return err
	}
	product, err := host.Resolve[*harnessproduct.Product](h, "harnessProduct")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return err
	}
	err = server.BindHarness(product, registry)
	if err != nil {
		return err
	}
	models, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return err
	}
	err = server.BindModels(models)
	if err != nil {
		return err
	}
	agentService, err := host.Resolve[*agents.Service](h, "agents")
	if err != nil {
		return err
	}
	err = server.BindAgents(agentService)
	if err != nil {
		return err
	}
	skillService, err := host.Resolve[skills.Skills](h, "skills")
	if err != nil {
		return err
	}
	err = server.BindSkills(skillService)
	if err != nil {
		return err
	}
	commandService, err := host.Resolve[commands.Commands](h, "commands")
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
