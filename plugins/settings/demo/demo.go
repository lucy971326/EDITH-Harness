package demo

import (
	nethttp "net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"harness/surface/web"
)

// 活对象。state 保存插件自身的演示配置。
// 状态归属说明：此状态仅在内存中持有，不进 Session，不造通用 Store；进程重启后重置。
type state struct {
	mu       sync.RWMutex
	nickname string
}

type demoSection struct {
	st *state
}

func newDemoSection(st *state) *demoSection {
	return &demoSection{st: st}
}

func (s *demoSection) Definition() web.SettingsSectionDefinition {
	return web.SettingsSectionDefinition{
		ID:    "demo",
		Title: "演示设置",
		Order: 10,
	}
}

func (s *demoSection) Render() (templ.Component, error) {
	s.st.mu.RLock()
	nickname := s.st.nickname
	s.st.mu.RUnlock()
	return DemoSettings(nickname, false), nil
}

type saveHandler struct {
	st *state
}

func newSaveHandler(st *state) *saveHandler {
	return &saveHandler{st: st}
}

func (h *saveHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	err := r.ParseForm()
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	nickname := strings.TrimSpace(r.FormValue("nickname"))
	if nickname == "" {
		nickname = "Explorer"
	}
	h.st.mu.Lock()
	h.st.nickname = nickname
	h.st.mu.Unlock()

	component := DemoSettings(nickname, true)
	templ.Handler(component).ServeHTTP(w, r)
}
