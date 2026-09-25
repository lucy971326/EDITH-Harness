package main

import (
	"errors"
	"fmt"
	"html"
	"os"

	"harness/clients/desktop"
	webclient "harness/clients/web"
	"harness/internal/backend"
	machinelocal "harness/internal/machine/local"

	"github.com/wailsapp/wails/v3/pkg/application"
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

	dataDir, err := backend.UserDataDir()
	if err != nil {
		return err
	}
	services, err := backend.Open(dataDir)
	if err != nil {
		// 启动失败时仍给用户一个可读的桌面窗口，尤其是 Web 已占用数据目录时。
		app := application.New(application.Options{Name: "Harness", Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true}})
		app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title: "Harness 无法启动", Width: 520, Height: 220,
			HTML: "<html><meta charset='utf-8'><body style='font:16px system-ui;padding:24px'><h2>Harness 无法启动</h2><p>如果 Web 或 Desktop 已在运行，请先关闭它。</p><pre style='white-space:pre-wrap'>" + html.EscapeString(err.Error()) + "</pre></body></html>",
		})
		return errors.Join(err, app.Run())
	}
	defer func() { result = errors.Join(result, services.Close()) }()

	assets, err := webclient.AssetsFS()
	if err != nil {
		return err
	}
	app := application.New(application.Options{
		Name:   "Harness",
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.HandleStream("rpc", func(conn *application.StreamConn) {
		services.Server.ServeStream(&desktop.Stream{Conn: conn})
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Harness", Width: 1440, Height: 900, MinWidth: 900, MinHeight: 600, URL: "/",
	})
	return app.Run()
}
