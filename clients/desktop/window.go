package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"harness/internal/persist"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 数据。Desktop 窗口的普通尺寸与位置；最大化不覆盖普通布局。
type WindowLayout struct {
	X, Y          int
	Width, Height int
	Maximized     bool
}

// 活对象。只保存 Desktop 窗口偏好，不持有业务状态。
type WindowPreferences struct {
	files  *persist.Files
	mu     sync.Mutex
	layout WindowLayout
	timer  *time.Timer
	ready  bool
	loaded bool
}

func NewWindowPreferences(dataDir string) (*WindowPreferences, error) {
	root, err := persist.NewFiles(dataDir)
	if err != nil {
		return nil, err
	}
	files, err := root.Scope("desktop")
	if err != nil {
		return nil, err
	}
	prefs := &WindowPreferences{files: files, layout: WindowLayout{Width: 1440, Height: 900}}
	data, err := files.Read("window.json")
	if err == nil {
		var saved WindowLayout
		if json.Unmarshal(data, &saved) == nil && saved.Width >= 900 && saved.Height >= 600 {
			prefs.layout = saved
			prefs.loaded = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return prefs, nil
}

func (p *WindowPreferences) Attach(app *application.App, window *application.WebviewWindow) {
	var firstShow sync.Once
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		firstShow.Do(func() {
			layout := p.layout
			if p.loaded {
				layout = fitLayout(layout, app.Screen.GetAll())
				window.SetSize(layout.Width, layout.Height)
				window.SetPosition(layout.X, layout.Y)
				if layout.Maximized {
					window.Maximise()
				}
			} else {
				layout.X, layout.Y = window.Position()
			}
			p.mu.Lock()
			p.layout = layout
			p.ready = true
			p.mu.Unlock()
		})
	})
	save := func(*application.WindowEvent) {
		p.mu.Lock()
		ready := p.ready
		p.mu.Unlock()
		if !ready {
			return
		}
		maximized := window.IsMaximised()
		var x, y, width, height int
		if !maximized {
			x, y = window.Position()
			width, height = window.Size()
		}
		p.mu.Lock()
		p.layout.Maximized = maximized
		if !maximized && width >= 900 && height >= 600 {
			p.layout.X, p.layout.Y = x, y
			p.layout.Width, p.layout.Height = width, height
		}
		if p.timer != nil {
			p.timer.Stop()
		}
		p.timer = time.AfterFunc(350*time.Millisecond, p.flush)
		p.mu.Unlock()
	}
	window.OnWindowEvent(events.Common.WindowDidMove, save)
	window.OnWindowEvent(events.Common.WindowDidResize, save)
}

func (p *WindowPreferences) Close() {
	p.mu.Lock()
	if p.timer != nil {
		p.timer.Stop()
	}
	ready := p.ready
	p.mu.Unlock()
	if ready {
		p.flush()
	}
}

func (p *WindowPreferences) flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := json.Marshal(p.layout)
	if err == nil {
		_ = p.files.Write("window.json", data)
	}
}

func fitLayout(saved WindowLayout, screens []*application.Screen) WindowLayout {
	if len(screens) == 0 {
		return saved
	}
	primary := screens[0].WorkArea
	for _, screen := range screens {
		if screen.IsPrimary {
			primary = screen.WorkArea
		}
		area := screen.WorkArea
		if saved.X < area.X+area.Width-100 && saved.X+saved.Width > area.X+100 &&
			saved.Y < area.Y+area.Height-100 && saved.Y+saved.Height > area.Y+100 {
			return clampLayout(saved, area)
		}
	}
	return clampLayout(WindowLayout{
		X:     primary.X + (primary.Width-saved.Width)/2,
		Y:     primary.Y + (primary.Height-saved.Height)/2,
		Width: saved.Width, Height: saved.Height, Maximized: saved.Maximized,
	}, primary)
}

func clampLayout(saved WindowLayout, area application.Rect) WindowLayout {
	if saved.Width > area.Width && area.Width >= 900 {
		saved.Width = area.Width
	}
	if saved.Height > area.Height && area.Height >= 600 {
		saved.Height = area.Height
	}
	if saved.X < area.X {
		saved.X = area.X
	}
	if saved.Y < area.Y {
		saved.Y = area.Y
	}
	if saved.X+saved.Width > area.X+area.Width {
		saved.X = area.X + area.Width - saved.Width
	}
	if saved.Y+saved.Height > area.Y+area.Height {
		saved.Y = area.Y + area.Height - saved.Height
	}
	return saved
}
