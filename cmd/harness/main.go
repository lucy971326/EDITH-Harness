package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"gopkg.in/yaml.v3"

	"harness/kernel/host"
	"harness/kernel/persist"
	"harness/kernel/session"
	chatproduct "harness/plugins/products/chat"
	"harness/surface/web"
)

// 数据。Config 是应用入口读取的静态组装配置。
type Config struct {
	DataDir string     `yaml:"dataDir"`
	Web     web.Config `yaml:"web"`
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

	h := host.NewHost()
	err = h.Install(&persist.Plugin{Dir: config.DataDir})
	if err != nil {
		return err
	}
	err = h.Install(&session.Plugin{})
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
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return config, nil
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
