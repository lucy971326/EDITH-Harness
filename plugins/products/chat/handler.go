package chat

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	nethttp "net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"

	"harness/kernel/agents"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/surface/web"
)

// 活对象。PageHandler 渲染 Chat 完整页面或 HTMX 主区域片段。
type PageHandler struct {
	web      web.Service
	product  web.Product
	sessions *session.Store
	settings settings.SessionSettingsStore
}

func newPageHandler(webService web.Service, product web.Product, sessions *session.Store, settingsStore settings.SessionSettingsStore) *PageHandler {
	return &PageHandler{web: webService, product: product, sessions: sessions, settings: settingsStore}
}

func (h *PageHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method == nethttp.MethodPost {
		h.createProject(w, r)
		return
	}

	data, err := h.pageData(r.PathValue("sessionID"))
	if err != nil {
		status := nethttp.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = nethttp.StatusNotFound
		}
		nethttp.Error(w, err.Error(), status)
		return
	}
	content := h.renderPage(r, data)
	templ.Handler(content).ServeHTTP(w, r)
}

func (h *PageHandler) renderPage(r *nethttp.Request, data pageView) templ.Component {
	navigation := ChatNavigation(data)
	content := ChatPage(data)
	if r.Header.Get("HX-Request") == "true" {
		return web.ProductFragment(navigation, content)
	}
	return web.Page(h.web.Products(), h.product.ID, navigation, content)
}

// 数据。Chat 页面展示的一组按工作区归类的会话。
type projectView struct {
	Name      string
	Workspace string
	Sessions  []sessionView
}

// 数据。Chat 页面展示的一本会话。
type sessionView struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

// 数据。Chat 页面渲染所需的全部数据。
type pageView struct {
	Projects  []projectView
	Selected  sessionView
	HasActive bool
	Error     string
}

var selectWorkspace = chooseWorkspace

func (h *PageHandler) pageData(selectedID string) (pageView, error) {
	metas, err := h.sessions.List()
	if err != nil {
		return pageView{}, fmt.Errorf("chat: list sessions: %w", err)
	}
	projects := make(map[string]*projectView)
	var selected sessionView
	found := selectedID == ""
	for _, meta := range metas {
		setup, err := h.settings.For(meta.ID)
		if err != nil {
			return pageView{}, fmt.Errorf("chat: settings for %q: %w", meta.ID, err)
		}
		item := sessionView{ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt}
		project := projects[setup.Workspace]
		if project == nil {
			project = &projectView{Name: filepath.Base(setup.Workspace), Workspace: setup.Workspace}
			projects[setup.Workspace] = project
		}
		project.Sessions = append(project.Sessions, item)
		if meta.ID == selectedID {
			selected = item
			found = true
		}
	}
	if !found {
		return pageView{}, fmt.Errorf("%w: session %q", os.ErrNotExist, selectedID)
	}
	out := pageView{Selected: selected, HasActive: selectedID != ""}
	for _, project := range projects {
		sort.Slice(project.Sessions, func(i, j int) bool {
			return project.Sessions[i].CreatedAt.After(project.Sessions[j].CreatedAt)
		})
		out.Projects = append(out.Projects, *project)
	}
	sort.Slice(out.Projects, func(i, j int) bool {
		return out.Projects[i].Name < out.Projects[j].Name
	})
	return out, nil
}

func (h *PageHandler) createProject(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspace, err := selectWorkspace(r.Context())
	if errors.Is(err, ErrCanceled) {
		nethttp.Redirect(w, r, "/chat", nethttp.StatusSeeOther)
		return
	}
	if err != nil {
		h.renderError(w, r, fmt.Errorf("选择工作区目录失败：%w", err))
		return
	}
	err = checkWorkspace(workspace)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	id, err := newSessionID()
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	_, err = h.sessions.Create(id)
	if err != nil {
		h.renderError(w, r, fmt.Errorf("创建会话失败：%w", err))
		return
	}
	setup := settings.SessionSettings{AgentID: agents.DefaultID, Workspace: workspace}
	err = h.settings.Put(id, setup)
	if err != nil {
		discardErr := h.sessions.DiscardEmpty(id)
		h.renderError(w, r, fmt.Errorf("保存会话设置失败：%w", errors.Join(err, discardErr)))
		return
	}
	nethttp.Redirect(w, r, "/chat/"+id, nethttp.StatusSeeOther)
}

func (h *PageHandler) renderError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	data, listErr := h.pageData("")
	if listErr != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	data.Error = err.Error()
	content := h.renderPage(r, data)
	w.WriteHeader(nethttp.StatusInternalServerError)
	templ.Handler(content).ServeHTTP(w, r)
}

func checkWorkspace(path string) error {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("工作区目录必须是绝对路径")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("工作区目录不可用：%w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("工作区路径不是目录")
	}
	return nil
}

func newSessionID() (string, error) {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		return "", fmt.Errorf("chat: make session id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
