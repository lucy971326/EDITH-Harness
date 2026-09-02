package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"harness/kernel/llm"
	"harness/kernel/runner"
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
	runner   *runner.Runner
	models   *llm.Client
	hub      *eventHub
}

func newPageHandler(
	webService web.Service,
	product web.Product,
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	runService *runner.Runner,
	modelService *llm.Client,
	hub *eventHub,
) *PageHandler {
	return &PageHandler{
		web: webService, product: product, sessions: sessions, settings: settingsStore,
		runner: runService, models: modelService, hub: hub,
	}
}

func (h *PageHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	switch {
	case r.Method == nethttp.MethodPost && r.URL.Path == "/chat/projects":
		h.createProject(w, r)
		return
	case r.Method == nethttp.MethodGet && strings.HasSuffix(r.URL.Path, "/history"):
		h.history(w, r)
		return
	case r.Method == nethttp.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
		h.events(w, r)
		return
	case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
		h.message(w, r)
		return
	case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
		h.stop(w, r)
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
	Projects        []projectView
	Selected        sessionView
	HasActive       bool
	Error           string
	Models          []llm.ModelChoice
	Model           string
	ReasoningEffort string
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
	if h.models != nil {
		out.Models = h.models.Models()
	}
	if selectedID != "" {
		setup, err := h.settings.For(selectedID)
		if err != nil {
			return pageView{}, fmt.Errorf("chat: settings for %q: %w", selectedID, err)
		}
		out.Model = setup.Model
		out.ReasoningEffort = setup.ReasoningEffort
	}
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

func (h *PageHandler) history(w nethttp.ResponseWriter, r *nethttp.Request) {
	sess, err := h.sessions.Get(r.PathValue("sessionID"))
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(sess.History()); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h *PageHandler) events(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionID")
	if _, err := h.sessions.Get(sessionID); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	flusher, ok := w.(nethttp.Flusher)
	if !ok {
		nethttp.Error(w, "streaming unsupported", nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	queue, unsubscribe := h.hub.subscribe(sessionID)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-queue:
			if !ok {
				return
			}
			body, err := json.Marshal(event)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: run\ndata: %s\n\n", body); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *PageHandler) message(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionID")
	if _, err := h.sessions.Get(sessionID); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		nethttp.Error(w, "消息不能为空", nethttp.StatusBadRequest)
		return
	}
	input := session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: text}}}
	switch r.FormValue("mode") {
	case "run":
		if err := h.selectModel(sessionID, r.FormValue("model"), r.FormValue("reasoningEffort")); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
			return
		}
		if err := h.startRun(sessionID, input); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusConflict)
			return
		}
	case "steer":
		if err := h.runner.Steer(sessionID, input); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusConflict)
			return
		}
	case "followup":
		if err := h.runner.FollowUp(sessionID, input); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusConflict)
			return
		}
	default:
		nethttp.Error(w, "未知消息模式", nethttp.StatusBadRequest)
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func (h *PageHandler) stop(w nethttp.ResponseWriter, r *nethttp.Request) {
	if err := h.runner.Stop(r.PathValue("sessionID")); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusConflict)
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func (h *PageHandler) startRun(sessionID string, input session.UserMessage) error {
	return h.runner.Start(context.Background(), sessionID, input)
}

func (h *PageHandler) selectModel(sessionID, model, effort string) error {
	setup, err := h.settings.For(sessionID)
	if err != nil {
		return err
	}
	if setup.Model != "" && setup.ReasoningEffort != "" {
		return nil
	}
	if model == "" || effort == "" {
		return fmt.Errorf("请先选择模型和思考档位")
	}
	for _, choice := range h.models.Models() {
		if choice.ID != model {
			continue
		}
		for _, available := range choice.ReasoningEfforts {
			if available == effort {
				setup.Model = model
				setup.ReasoningEffort = effort
				return h.settings.Put(sessionID, setup)
			}
		}
	}
	return fmt.Errorf("模型或思考档位不可用")
}

func (h *PageHandler) renderError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	if h.web == nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
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
