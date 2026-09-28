package main

import (
	"errors"
	"fmt"
	"html"
	"os"
	"sync"

	clientassets "harness/clients"
	"harness/clients/desktop"
	"harness/internal/backend"
	machinelocal "harness/internal/machine/local"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	assets, err := clientassets.AssetsFS()
	if err != nil {
		return err
	}
	// Wails 的第二实例判定在 New 中完成，必须先于后台的数据目录锁。
	var wakeMu sync.Mutex
	var wakeWindow *application.WebviewWindow
	var wakeReady, wakePending bool
	var lifecycle *desktop.Lifecycle
	app := application.New(application.Options{
		Name:   "Harness",
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.edith.harness.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				wakeMu.Lock()
				if !wakeReady {
					wakePending = true
					wakeMu.Unlock()
					return
				}
				window := wakeWindow
				wakeMu.Unlock()
				if window != nil {
					window.Show()
					window.UnMinimise()
					window.Focus()
				}
			},
		},
		ShouldQuit: func() bool {
			if lifecycle == nil {
				return true
			}
			return lifecycle.ShouldQuit()
		},
	})
	attachWake := func(window *application.WebviewWindow) {
		wakeMu.Lock()
		wakeWindow = window
		wakeMu.Unlock()
		window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
			wakeMu.Lock()
			wakeReady = true
			pending := wakePending
			wakePending = false
			wakeMu.Unlock()
			if pending {
				window.UnMinimise()
				window.Focus()
			}
		})
	}
	services, err := backend.Open(dataDir)
	if err != nil {
		// 启动失败时仍给用户一个可读的桌面窗口，尤其是 Web 已占用数据目录时。
		window := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title: "Harness 无法启动", Width: 520, Height: 220,
			HTML: "<html><meta charset='utf-8'><body style='font:16px system-ui;padding:24px'><h2>Harness 无法启动</h2><p>如果 Web 或 Desktop 已在运行，请先关闭它。</p><pre style='white-space:pre-wrap'>" + html.EscapeString(err.Error()) + "</pre></body></html>",
		})
		attachWake(window)
		return errors.Join(err, app.Run())
	}
	defer func() { result = errors.Join(result, services.Close()) }()

	preferences, err := desktop.NewWindowPreferences(dataDir)
	if err != nil {
		return err
	}
	defer preferences.Close()
	app.HandleStream("rpc", func(conn *application.StreamConn) {
		services.Server.ServeStream(&desktop.Stream{Conn: conn})
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Harness", Width: 1440, Height: 900, MinWidth: 900, MinHeight: 600, URL: "/",
	})
	preferences.Attach(app, window)
	lifecycle = desktop.NewLifecycle(app, window, services.HasActiveRuns)
	attachWake(window)
	return app.Run()
}
