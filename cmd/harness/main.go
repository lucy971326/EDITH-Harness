package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"gopkg.in/yaml.v3"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/skills"
	"harness/kernel/tools"
	"harness/plugins/kernel/loops/react"
	machinelocal "harness/plugins/kernel/machine/local"
	bashtool "harness/plugins/kernel/tools/bash"
	edittool "harness/plugins/kernel/tools/edit"
	readtool "harness/plugins/kernel/tools/read"
	writetool "harness/plugins/kernel/tools/write"
	chatproduct "harness/plugins/web/chat"
	composerdemo "harness/plugins/web/chat/composer/demo"
	messagefork "harness/plugins/web/chat/message/fork"
	paneldemo "harness/plugins/web/chat/sidepanel/demo"
	webdemo "harness/plugins/web/demo"
	settingsdemo "harness/plugins/web/settings/demo"
	"harness/surface/web"
)

// 数据。Config 是应用入口读取的静态组装配置。
type Config struct {
	Web web.Config `yaml:"web"`
}

func main() {
	err := run("harness.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	config, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	dataDir, err := userDataDir()
	if err != nil {
		return err
	}

	h := host.NewHost()
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
	err = h.Install(agents.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(runner.NewPlugin())
	if err != nil {
		return err
	}
	webPlugin := web.NewPlugin(config.Web)
	err = h.Install(webPlugin)
	if err != nil {
		return err
	}
	err = h.Install(chatproduct.NewPlugin())
	if err != nil {
		return err
	}
	err = h.Install(webdemo.New())
	if err != nil {
		return err
	}
	err = h.Install(paneldemo.New())
	if err != nil {
		return err
	}
	err = h.Install(composerdemo.New())
	if err != nil {
		return err
	}
	err = h.Install(messagefork.New())
	if err != nil {
		return err
	}
	err = h.Install(settingsdemo.New())
	if err != nil {
		return err
	}

	url := webPlugin.URL()
	err = openBrowser(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法自动打开浏览器：%v\n", err)
	}
	fmt.Printf("Harness 已启动：%s\n", url)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	err = h.Close()
	if err != nil {
		return err
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	err = decoder.Decode(&config)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return config, nil
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
