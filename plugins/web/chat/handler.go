package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"

	"harness/kernel/agents"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/surface/web"
	"harness/surface/web/runview"
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
	registry Service
	createMu sync.Mutex
}

func newPageHandler(
	webService web.Service,
	product web.Product,
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	runService *runner.Runner,
	modelService *llm.Client,
	hub *eventHub,
	registry Service,
) *PageHandler {
	return &PageHandler{
		web: webService, product: product, sessions: sessions, settings: settingsStore,
		runner: runService, models: modelService, hub: hub, registry: registry,
	}
}

func (h *PageHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	switch {
	case r.Method == nethttp.MethodPost && r.URL.Path == "/chat/projects":
		h.createProject(w, r)
		return
	case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/sessions"):
		h.createSessionInProject(w, r)
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
	case r.Method == nethttp.MethodPost && strings.Contains(r.URL.Path, "/message-actions/"):
		h.messageAction(w, r)
		return
	case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
		h.stop(w, r)
		return
	case r.Method == nethttp.MethodGet && strings.Contains(r.URL.Path, "/panels/"):
		h.panel(w, r)
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
		return web.ProductFragment(h.web.Products(), h.product.ID, navigation, content)
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
	Projects           []projectView
	Selected           sessionView
	HasActive          bool
	Error              string
	Models             []llm.ModelChoice
	Model              string
	ReasoningEffort    string
	Panels             []PanelDefinition
	Docks              []dockView
	MessageActionsJSON string
	ComposerActions    []composerActionView
}

// 数据。Chat 固定外壳与 Dock 插件内容组成的页面条目。
type dockView struct {
	Definition DockDefinition
	Content    templ.Component
}

// 数据。Chat 输入工具栏与插件自绘内容组成的页面条目。
type composerActionView struct {
	Definition ComposerActionDefinition
	Content    templ.Component
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
	out := pageView{Selected: selected, HasActive: selectedID != "", MessageActionsJSON: "[]"}
	if h.registry != nil {
		out.Panels = h.registry.Panels()
		actionsJSON, err := json.Marshal(h.registry.MessageActions())
		if err != nil {
			return pageView{}, fmt.Errorf("chat: encode message actions: %w", err)
		}
		out.MessageActionsJSON = string(actionsJSON)
		if selectedID != "" {
			setup, err := h.settings.For(selectedID)
			if err != nil {
				return pageView{}, fmt.Errorf("chat: settings for %q: %w", selectedID, err)
			}
			out.Docks = h.dockViews(DockContext{SessionID: selectedID, Workspace: setup.Workspace})
			out.ComposerActions = h.composerActionViews(ComposerActionContext{SessionID: selectedID, Workspace: setup.Workspace})
		}
	}
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

func (h *PageHandler) dockViews(dockContext DockContext) []dockView {
	if h.registry == nil {
		return nil
	}
	definitions := h.registry.Docks()
	views := make([]dockView, 0, len(definitions))
	for _, definition := range definitions {
		content, err := h.renderDock(dockContext, definition.ID)
		if err != nil {
			content = DockRenderError()
		}
		views = append(views, dockView{Definition: definition, Content: content})
	}
	return views
}

func (h *PageHandler) composerActionViews(composerContext ComposerActionContext) []composerActionView {
	if h.registry == nil {
		return nil
	}
	definitions := h.registry.ComposerActions()
	views := make([]composerActionView, 0, len(definitions))
	for _, definition := range definitions {
		action, ok := h.registry.ComposerAction(definition.ID)
		if !ok {
			continue
		}
		content, err := action.Render(composerContext)
		if err != nil || content == nil {
			// 单个 Action 渲染失败或返回空组件则优雅跳过，隔离错误
			continue
		}
		views = append(views, composerActionView{Definition: definition, Content: content})
	}
	return views
}

func (h *PageHandler) renderDock(dockContext DockContext, dockID string) (templ.Component, error) {
	if h.registry == nil {
		return nil, fmt.Errorf("chat: dock registry unavailable")
	}
	dock, ok := h.registry.Dock(dockID)
	if !ok {
		return nil, fmt.Errorf("chat: dock %q not registered", dockID)
	}
	return dock.Render(dockContext)
}

func renderComponent(component templ.Component) (string, error) {
	var output bytes.Buffer
	err := component.Render(context.Background(), &output)
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

func (h *PageHandler) panel(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionID")
	if _, err := h.sessions.Get(sessionID); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	if h.registry == nil {
		nethttp.NotFound(w, r)
		return
	}
	panel, ok := h.registry.Panel(r.PathValue("panelType"))
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	instanceKey := strings.TrimSpace(r.URL.Query().Get("instance"))
	if instanceKey == "" {
		nethttp.Error(w, "缺少面板实例", nethttp.StatusBadRequest)
		return
	}
	setup, err := h.settings.For(sessionID)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	content, err := panel.Render(PanelContext{SessionID: sessionID, Workspace: setup.Workspace}, PanelTab{
		Type:        r.PathValue("panelType"),
		InstanceKey: instanceKey,
	})
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	templ.Handler(content).ServeHTTP(w, r)
}

func (h *PageHandler) messageAction(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionID")
	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	if h.registry == nil {
		nethttp.NotFound(w, r)
		return
	}
	action, ok := h.registry.MessageAction(r.PathValue("actionID"))
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	err = r.ParseForm()
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	target := MessageActionTarget{
		CardType:        MessageCardType(r.FormValue("cardType")),
		EntryID:         strings.TrimSpace(r.FormValue("entryID")),
		RunID:           strings.TrimSpace(r.FormValue("runID")),
		BoundaryEntryID: strings.TrimSpace(r.FormValue("boundaryEntryID")),
	}
	entries := sess.Entries()
	err = validateMessageActionTarget(action.Definition(), entries, target)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	if target.CardType == MessageCardAssistant && h.runner != nil {
		if state, running := h.runner.State(sessionID); running && state.RunID == target.RunID {
			nethttp.Error(w, "助手回答仍在生成", nethttp.StatusConflict)
			return
		}
	}
	result, err := action.Execute(r.Context(), MessageActionContext{
		SessionID: sessionID,
		Entries:   entries,
		Target:    target,
	})
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		return
	}
}

func validateMessageActionTarget(definition MessageActionDefinition, entries []session.Entry, target MessageActionTarget) error {
	if !actionSupportsTarget(definition, target.CardType) {
		return fmt.Errorf("chat: action %q does not support %q cards", definition.ID, target.CardType)
	}
	switch target.CardType {
	case MessageCardUser:
		if target.EntryID == "" || target.RunID != "" || target.BoundaryEntryID != "" {
			return fmt.Errorf("chat: invalid user message action target")
		}
		entry, ok := entryByID(entries, target.EntryID)
		if !ok || entry.Message.Role != session.RoleUser {
			return fmt.Errorf("chat: user message action target not found")
		}
	case MessageCardAssistant:
		if target.EntryID != "" || target.RunID == "" || target.BoundaryEntryID == "" {
			return fmt.Errorf("chat: invalid assistant message action target")
		}
		if assistantSegmentStart(entries, target.RunID, target.BoundaryEntryID) < 0 {
			return fmt.Errorf("chat: assistant message action target not found")
		}
	default:
		return fmt.Errorf("chat: unknown message action card type %q", target.CardType)
	}
	return nil
}

func actionSupportsTarget(definition MessageActionDefinition, target MessageCardType) bool {
	for _, supported := range definition.Targets {
		if supported == target {
			return true
		}
	}
	return false
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
	h.createSession(w, r, workspace)
}

func (h *PageHandler) createSessionInProject(w nethttp.ResponseWriter, r *nethttp.Request) {
	setup, err := h.settings.For(r.PathValue("sessionID"))
	if err != nil {
		nethttp.Error(w, "未找到该项目", nethttp.StatusNotFound)
		return
	}
	h.createSession(w, r, setup.Workspace)
}

func (h *PageHandler) createSession(w nethttp.ResponseWriter, r *nethttp.Request, workspace string) {
	h.createMu.Lock()
	defer h.createMu.Unlock()

	existingID, err := h.emptySessionIn(workspace)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	if existingID != "" {
		nethttp.Redirect(w, r, "/chat/"+existingID, nethttp.StatusSeeOther)
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

func (h *PageHandler) emptySessionIn(workspace string) (string, error) {
	metas, err := h.sessions.List()
	if err != nil {
		return "", fmt.Errorf("读取会话失败：%w", err)
	}
	for _, meta := range metas {
		setup, err := h.settings.For(meta.ID)
		if err != nil {
			return "", fmt.Errorf("读取会话设置失败：%w", err)
		}
		if setup.Workspace != workspace {
			continue
		}
		sess, err := h.sessions.Get(meta.ID)
		if err != nil {
			return "", fmt.Errorf("读取会话失败：%w", err)
		}
		if len(sess.Entries()) == 0 {
			return meta.ID, nil
		}
	}
	return "", nil
}

func (h *PageHandler) history(w nethttp.ResponseWriter, r *nethttp.Request) {
	sess, err := h.sessions.Get(r.PathValue("sessionID"))
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	snapshot := runview.Snapshot{Entries: sess.Entries(), Runs: []runner.RunState{}}
	if state, ok := h.runner.State(r.PathValue("sessionID")); ok {
		snapshot.Runs = []runner.RunState{state}
	}
	if err := json.NewEncoder(w).Encode(snapshot); err != nil {
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
	setup, err := h.settings.For(sessionID)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	queue, unsubscribe := h.hub.subscribe(sessionID)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	dockContext := DockContext{SessionID: sessionID, Workspace: setup.Workspace}
	for _, definition := range h.registry.Docks() {
		if !h.writeDockEvent(w, flusher, dockContext, definition.ID) {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-queue:
			if !ok {
				return
			}
			if event.run != nil {
				body, err := json.Marshal(*event.run)
				if err != nil {
					return
				}
				if err := writeSSE(w, "run", string(body)); err != nil {
					return
				}
				flusher.Flush()
				continue
			}
			if event.dock != nil {
				if !h.writeDockEvent(w, flusher, dockContext, event.dock.DockID) {
					return
				}
			}
		}
	}
}

func (h *PageHandler) writeDockEvent(w nethttp.ResponseWriter, flusher nethttp.Flusher, dockContext DockContext, dockID string) bool {
	if h.registry == nil {
		return true
	}
	if _, ok := h.registry.Dock(dockID); !ok {
		return true
	}
	content, err := h.renderDock(dockContext, dockID)
	if err != nil {
		content = DockRenderError()
	}
	body, err := renderComponent(content)
	if err != nil {
		return true
	}
	if err := writeSSE(w, "dock-"+dockID, body); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func writeSSE(w nethttp.ResponseWriter, event string, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}

func (h *PageHandler) message(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionID")
	if _, err := h.sessions.Get(sessionID); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusNotFound)
		return
	}
	contentType := r.Header.Get("Content-Type")
	var err error
	if strings.HasPrefix(contentType, "multipart/form-data") {
		err = r.ParseMultipartForm(32 << 20)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
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
	return session.NewID()
}
