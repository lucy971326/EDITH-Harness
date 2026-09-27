package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	clientassets "harness/clients"
	webclient "harness/clients/web"
	"harness/internal/backend"
	machinelocal "harness/internal/machine/local"
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
	noBrowser := len(os.Args) == 2 && os.Args[1] == "--no-browser"
	if len(os.Args) > 1 && !noBrowser {
		return fmt.Errorf("usage: harness [--no-browser]")
	}

	dataDir, err := backend.UserDataDir()
	if err != nil {
		return err
	}
	services, err := backend.Open(dataDir)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, services.Close()) }()
	assets, err := clientassets.AssetsFS()
	if err != nil {
		return err
	}
	web, err := webclient.New(services.Server, http.FileServerFS(assets))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, web.Close()) }()
	url, err := web.Listen("127.0.0.1:8888")
	if err != nil {
		return err
	}
	if !noBrowser {
		err = openBrowser(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "无法自动打开浏览器：%v\n", err)
		}
	}
	fmt.Printf("Harness 后台已启动：%s\n", url)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
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
