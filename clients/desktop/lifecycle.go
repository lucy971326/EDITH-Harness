package desktop

import (
	_ "embed"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Icon 是 Windows 应用和各平台托盘使用的 EDITH 图标；macOS 应用图标来自 ICNS。
//
//go:embed edith-icon.png
var Icon []byte

type closeRequest struct {
	id       uint64
	phase    string
	running  bool
	terminal bool
	dirty    bool
	timer    *time.Timer
}

type closeState struct {
	running, terminalOpen, dirty, saving bool
}

func closeAction(state closeState) string {
	if state.saving {
		return "blocked"
	}
	if state.running || state.terminalOpen || state.dirty {
		return "confirm"
	}
	return "quit"
}

// 活对象。Desktop 窗口、托盘和退出流程；业务运行状态只从注入的查询读取。
type Lifecycle struct {
	app       *application.App
	window    *application.WebviewWindow
	activeRun func() bool
	mu        sync.Mutex
	nextID    uint64
	pending   *closeRequest
	quitting  atomic.Bool
}

func NewLifecycle(app *application.App, window *application.WebviewWindow, activeRun func() bool) *Lifecycle {
	l := &Lifecycle{app: app, window: window, activeRun: activeRun}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		l.window.Hide()
	})
	app.Event.On("desktop-close-state", l.onState)
	app.Event.On("desktop-exit-decision", l.onDecision)
	tray := app.SystemTray.New()
	tray.SetIcon(Icon)
	tray.SetTooltip("EDITH")
	menu := app.NewMenu()
	menu.Add("显示窗口").OnClick(func(*application.Context) { l.Show() })
	menu.Add("退出 EDITH").OnClick(func(*application.Context) { l.request() })
	tray.SetMenu(menu)
	tray.OnClick(l.Show)
	return l
}

func (l *Lifecycle) Show() {
	l.window.Show()
	l.window.UnMinimise()
	l.window.Focus()
}

// ShouldQuit 拦截操作系统的退出入口；确认完成后直接调用 app.Quit。
func (l *Lifecycle) ShouldQuit() bool {
	if l.quitting.Load() {
		return true
	}
	l.request()
	return false
}

func (l *Lifecycle) request() {
	l.mu.Lock()
	if l.pending != nil {
		l.mu.Unlock()
		return
	}
	l.nextID++
	request := &closeRequest{id: l.nextID, phase: "state"}
	l.pending = request
	request.timer = time.AfterFunc(2*time.Second, func() { l.fallback(request.id) })
	l.mu.Unlock()
	l.window.EmitEvent("desktop-close-request", map[string]any{"id": request.id})
}

func (l *Lifecycle) onState(event *application.CustomEvent) {
	data, ok := event.Data.(map[string]any)
	if !ok {
		return
	}
	id, ok := data["id"].(float64)
	if !ok {
		return
	}
	l.mu.Lock()
	request := l.pending
	if request == nil || request.id != uint64(id) || request.phase != "state" {
		l.mu.Unlock()
		return
	}
	request.timer.Stop()
	request.running = l.activeRun()
	terminal, _ := data["terminalOpen"].(bool)
	dirty, _ := data["dirty"].(bool)
	saving, _ := data["saving"].(bool)
	request.terminal, request.dirty = terminal, dirty
	state := closeState{running: request.running, terminalOpen: terminal, dirty: dirty, saving: saving}
	action := closeAction(state)
	if action != "confirm" {
		l.pending = nil
	} else {
		request.phase = "confirm"
	}
	l.mu.Unlock()

	switch action {
	case "blocked":
		l.Show()
		l.app.Dialog.Info().SetTitle("正在保存").SetMessage("请等待保存完成后再退出。").AttachToWindow(l.window).Show()
		return
	case "quit":
		l.quit()
		return
	}
	l.Show()
	l.window.EmitEvent("desktop-exit-confirm", map[string]any{
		"id": request.id, "running": request.running, "terminalOpen": terminal, "dirty": dirty,
	})
}

func (l *Lifecycle) onDecision(event *application.CustomEvent) {
	data, ok := event.Data.(map[string]any)
	if !ok {
		return
	}
	id, ok := data["id"].(float64)
	if !ok {
		return
	}
	confirmed, _ := data["confirmed"].(bool)
	l.mu.Lock()
	request := l.pending
	if request == nil || request.id != uint64(id) || request.phase != "confirm" {
		l.mu.Unlock()
		return
	}
	l.pending = nil
	l.mu.Unlock()
	if !confirmed {
		return
	}
	saving, _ := data["saving"].(bool)
	if saving {
		l.Show()
		l.app.Dialog.Info().SetTitle("正在保存").SetMessage("请等待保存完成后再退出。").AttachToWindow(l.window).Show()
		return
	}
	terminal, _ := data["terminalOpen"].(bool)
	dirty, _ := data["dirty"].(bool)
	// 确认弹窗打开期间可能刚启动新任务；重新询问，不能静默取消它。
	if !request.running && l.activeRun() || !request.terminal && terminal || !request.dirty && dirty {
		l.request()
		return
	}
	l.quit()
}

func (l *Lifecycle) fallback(id uint64) {
	l.mu.Lock()
	if l.pending == nil || l.pending.id != id || l.pending.phase != "state" {
		l.mu.Unlock()
		return
	}
	l.pending = nil
	l.mu.Unlock()
	l.nativeConfirm("界面未响应，无法确认未保存内容。退出将停止运行任务和终端，并丢弃未保存内容。")
}

func (l *Lifecycle) nativeConfirm(message string) {
	l.Show()
	dialog := l.app.Dialog.Question().SetTitle("退出 EDITH？").SetMessage(message).AttachToWindow(l.window)
	dialog.AddButton("继续使用").SetAsCancel()
	dialog.AddButton("退出 EDITH").OnClick(l.quit)
	dialog.Show()
}

func (l *Lifecycle) quit() {
	l.quitting.Store(true)
	l.app.Quit()
}
