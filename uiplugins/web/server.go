package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"

	"harness/agents"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

//go:embed static/*
var staticFiles embed.FS

var markdown = goldmark.New()

// service 把领域能力组合成一个只监听本机的网页入口。
type service struct {
	projects projects.Service
	presets  presets.Service
	books    *session.Store
	agents   agents.Service
	llm      *llm.Service
	tools    *tools.Registry
	updates  *updateHub
	picker   directoryPicker

	mu     sync.Mutex
	server *http.Server
}

func newService(projectService projects.Service, presetService presets.Service, books *session.Store, agentService agents.Service, llmService *llm.Service, toolRegistry *tools.Registry, updates *updateHub, picker directoryPicker) *service {
	return &service{
		projects: projectService,
		presets:  presetService,
		books:    books,
		agents:   agentService,
		llm:      llmService,
		tools:    toolRegistry,
		updates:  updates,
		picker:   picker,
	}
}

// Run 启动本机网页服务，直到上下文取消或 App 收摊。
func (s *service) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return fmt.Errorf("Web UI 无法监听 127.0.0.1:8080：%w", err)
	}
	server := &http.Server{Handler: s.routes()}
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("Web UI 已经启动")
	}
	s.server = server
	s.mu.Unlock()

	fmt.Println("Web UI 已启动：http://127.0.0.1:8080")
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("Web UI 已停止：%w", err)
}

// Close 关闭正在监听的 HTTP 服务；尚未启动时是空操作。
func (s *service) Close() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /fragments/chat", s.handleChatFragment)
	mux.HandleFunc("POST /projects/pick", s.handlePickProject)
	mux.HandleFunc("GET /presets", s.handlePresetList)
	mux.HandleFunc("GET /presets/new", s.handleNewPreset)
	mux.HandleFunc("GET /presets/edit", s.handleEditPreset)
	mux.HandleFunc("POST /presets", s.handleCreatePreset)
	mux.HandleFunc("POST /presets/update", s.handleUpdatePreset)
	mux.HandleFunc("POST /presets/archive", s.handleArchivePreset)
	mux.HandleFunc("POST /messages", s.handleSubmitMessage)
	mux.HandleFunc("POST /sessions/model", s.handleSelectModel)
	mux.HandleFunc("POST /sessions/cancel", s.handleCancelSession)
	mux.HandleFunc("GET /events", s.handleEvents)
	return s.sameOriginWrites(mux)
}

func (s *service) handleCancelSession(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "取消表单读取失败", http.StatusBadRequest)
		return
	}
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	conversation, err := s.agents.GetSession(sessionID)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, "当前会话没有在运行")
		return
	}
	conversation.Cancel()
	data, err := s.pageData(projectID, sessionID, "", "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) sameOriginWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			next.ServeHTTP(writer, request)
			return
		}
		origin := request.Header.Get("Origin")
		if origin != "" && !matchesRequestOrigin(origin, request.Host) {
			http.Error(writer, "跨源写请求被拒绝", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func matchesRequestOrigin(origin string, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" && parsed.Host == host
}

func (s *service) handleHome(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), request.URL.Query().Get("error"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, Page(data))
}

func (s *service) handleChatFragment(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) handlePickProject(writer http.ResponseWriter, request *http.Request) {
	root, err := s.picker.Pick(request.Context())
	if errors.Is(err, errDirectoryPickCancelled) {
		http.Redirect(writer, request, "/", http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	project, err := s.projects.Create(root)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/?project="+url.QueryEscape(project.ID)+"&draft=1", http.StatusSeeOther)
}

func (s *service) handlePresetList(writer http.ResponseWriter, request *http.Request) {
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetListPage(data))
}

func (s *service) handleNewPreset(writer http.ResponseWriter, request *http.Request) {
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, presets.Preset{}))
}

func (s *service) handleEditPreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := s.presets.Get(request.URL.Query().Get("id"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, preset))
}

func (s *service) handleCreatePreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := presetFromForm(request)
	if err == nil {
		err = s.validatePresetChoices(preset)
	}
	if err == nil {
		err = s.presets.Create(preset)
	}
	if err != nil {
		s.renderPresetFormError(writer, request, preset, err)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) handleUpdatePreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := presetFromForm(request)
	if err == nil {
		err = s.validatePresetChoices(preset)
	}
	if err == nil {
		err = s.presets.Update(preset)
	}
	if err != nil {
		s.renderPresetFormError(writer, request, preset, err)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) validatePresetChoices(preset presets.Preset) error {
	if preset.ID == "" {
		return fmt.Errorf("Agent 模式必须有名称")
	}
	installed := make(map[string]bool)
	for _, name := range s.tools.Names() {
		installed[name] = true
	}
	for _, name := range preset.Tools {
		if !installed[name] {
			return fmt.Errorf("工具 %s 没有安装", name)
		}
	}
	return nil
}

func (s *service) handleArchivePreset(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err == nil {
		err = s.presets.Archive(request.Form.Get("id"))
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) renderPresetFormError(writer http.ResponseWriter, request *http.Request, preset presets.Preset, cause error) {
	data, err := s.presetPageData(cause.Error())
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, preset))
}

func (s *service) handleSubmitMessage(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "消息表单读取失败", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(request.Form.Get("text"))
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	presetID := request.Form.Get("preset_id")
	selection, selectionErr := s.parseSelection(request.Form.Get("model_selection"))
	if selectionErr != nil {
		s.respondMessageError(writer, request, projectID, sessionID, selectionErr.Error())
		return
	}
	if text == "" {
		s.respondMessageError(writer, request, projectID, sessionID, "消息不能为空")
		return
	}

	if sessionID == "" {
		conversation, startErr := s.agents.StartSession(agents.StartInput{
			ProjectID: projectID,
			PresetID:  presetID,
			Title:     titleFromMessage(text),
			Model:     selection,
		})
		if startErr != nil {
			s.respondMessageError(writer, request, projectID, "", startErr.Error())
			return
		}
		sendErr := conversation.SubmitFollowup(text)
		if sendErr != nil {
			s.respondMessageError(writer, request, projectID, conversation.SessionID(), sendErr.Error())
			return
		}
		rememberErr := s.projects.RememberPreset(projectID, presetID)
		if rememberErr != nil {
			fmt.Printf("Web UI：记住项目模式失败：%v\n", rememberErr)
		}
		rememberErr = s.projects.RememberModel(projectID, selection)
		if rememberErr != nil {
			fmt.Printf("Web UI：记住项目模型失败：%v\n", rememberErr)
		}
		sessionID = conversation.SessionID()
		s.redirectAfterNewSession(writer, request, projectID, sessionID)
		return
	}

	conversation, openErr := s.agents.OpenSession(sessionID)
	if openErr != nil {
		s.respondMessageError(writer, request, projectID, sessionID, openErr.Error())
		return
	}
	err = conversation.SelectModel(selection)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	err = s.projects.RememberModel(projectID, selection)
	if err != nil {
		fmt.Printf("Web UI：记住项目模型失败：%v\n", err)
	}
	err = conversation.SubmitFollowup(text)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	data, dataErr := s.pageData(projectID, sessionID, "", "")
	if dataErr != nil {
		http.Error(writer, dataErr.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, ChatPanel(data))
}

// handleSelectModel 立刻切换已打开会话的下一轮模型，并把它记进账本。
func (s *service) handleSelectModel(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "模型表单读取失败", http.StatusBadRequest)
		return
	}
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	selection, err := s.parseSelection(request.Form.Get("model_selection"))
	if err == nil {
		conversation, openErr := s.agents.OpenSession(sessionID)
		if openErr != nil {
			err = openErr
		} else {
			err = conversation.SelectModel(selection)
		}
	}
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	err = s.projects.RememberModel(projectID, selection)
	if err != nil {
		fmt.Printf("Web UI：记住项目模型失败：%v\n", err)
	}
	data, err := s.pageData(projectID, sessionID, "", "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) redirectAfterNewSession(writer http.ResponseWriter, request *http.Request, projectID string, sessionID string) {
	target := "/?project=" + url.QueryEscape(projectID) + "&session=" + url.QueryEscape(sessionID)
	if request.Header.Get("HX-Request") == "true" {
		writer.Header().Set("HX-Redirect", target)
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(writer, request, target, http.StatusSeeOther)
}

func (s *service) respondMessageError(writer http.ResponseWriter, request *http.Request, projectID string, sessionID string, message string) {
	data, err := s.pageData(projectID, sessionID, "1", message)
	if err != nil {
		http.Error(writer, message, http.StatusBadRequest)
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) handleEvents(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(writer, "缺少 session", http.StatusBadRequest)
		return
	}
	updates, unregister := s.updates.Subscribe(sessionID)
	defer unregister()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "当前服务器不支持事件流", http.StatusInternalServerError)
		return
	}
	fmt.Fprint(writer, "event: ready\ndata: ok\n\n")
	flusher.Flush()
	for {
		select {
		case _, open := <-updates:
			if !open {
				return
			}
			fmt.Fprint(writer, "event: refresh\ndata: now\n\n")
			flusher.Flush()
		case <-request.Context().Done():
			return
		}
	}
}

func (s *service) pageData(projectID string, sessionID string, draft string, message string) (PageData, error) {
	listedProjects, err := s.projects.List()
	if err != nil {
		return PageData{}, err
	}
	data := PageData{Projects: listedProjects, Error: message}
	data.Providers = s.llm.Providers()
	for _, project := range listedProjects {
		if project.Archived {
			continue
		}
		headers, listErr := s.projects.ListSessions(project.ID)
		if listErr != nil {
			return PageData{}, listErr
		}
		sort.Slice(headers, func(i int, j int) bool {
			return headers[i].CreatedAt.After(headers[j].CreatedAt)
		})
		data.ProjectTrees = append(data.ProjectTrees, ProjectTree{Project: project, Sessions: headers})
	}
	if projectID == "" {
		for _, project := range listedProjects {
			if !project.Archived {
				projectID = project.ID
				break
			}
		}
	}
	if projectID == "" {
		return data, nil
	}
	project, err := s.projects.Get(projectID)
	if err != nil {
		return PageData{}, err
	}
	data.Project = project
	data.HasProject = true
	data.Sessions, err = s.projects.ListSessions(projectID)
	if err != nil {
		return PageData{}, err
	}
	sort.Slice(data.Sessions, func(i int, j int) bool {
		return data.Sessions[i].CreatedAt.After(data.Sessions[j].CreatedAt)
	})
	data.Presets, err = s.presets.List()
	if err != nil {
		return PageData{}, err
	}
	data.SelectedPreset = pickPreset(data.Presets, project.LastPresetID)
	data.SelectedModel = project.LastModel
	if data.SelectedModel.Provider == "" {
		data.SelectedModel, err = s.llm.DefaultSelection()
		if err != nil {
			return PageData{}, err
		}
	}
	data.HasPreset = data.SelectedPreset.ID != ""

	if sessionID == "" {
		data.Draft = draft == "1"
		return data, nil
	}
	header, events, err := s.books.Read(sessionID)
	if err != nil {
		return PageData{}, err
	}
	if header.ProjectID != projectID {
		return PageData{}, fmt.Errorf("会话 %s 不属于项目 %s", sessionID, projectID)
	}
	data.Header = header
	data.HasSession = true
	data.Chat = projectEvents(events)
	if selected, found := latestModelSelection(events); found {
		data.SelectedModel = selected
	}
	locked, lockErr := s.presets.GetRevision(header.PresetID, header.PresetRevision)
	if lockErr == nil {
		data.SelectedPreset = locked
		data.HasPreset = true
	}
	return data, nil
}

func (s *service) presetPageData(message string) (PresetPageData, error) {
	listed, err := s.presets.List()
	if err != nil {
		return PresetPageData{}, err
	}
	return PresetPageData{
		Presets: listed,
		Tools:   s.tools.Names(),
		Error:   message,
	}, nil
}

func pickPreset(listed []presets.Preset, preferred string) presets.Preset {
	for _, preset := range listed {
		if preset.ID == preferred && !preset.Archived {
			return preset
		}
	}
	for _, preset := range listed {
		if !preset.Archived {
			return preset
		}
	}
	return presets.Preset{}
}

func presetFromForm(request *http.Request) (presets.Preset, error) {
	err := request.ParseForm()
	if err != nil {
		return presets.Preset{}, err
	}
	preset := presets.Preset{
		ID:           strings.TrimSpace(request.Form.Get("id")),
		SystemPrompt: strings.TrimSpace(request.Form.Get("system_prompt")),
		Tools:        request.Form["tools"],
	}
	return preset, nil
}

func (s *service) parseSelection(value string) (llm.Selection, error) {
	parts := strings.Split(value, "\x1f")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return llm.Selection{}, fmt.Errorf("请选择一个完整模型")
	}
	selection := llm.Selection{Provider: parts[0], Model: parts[1], Thinking: parts[2]}
	err := s.llm.Validate(selection)
	if err != nil {
		return llm.Selection{}, err
	}
	return selection, nil
}

func latestModelSelection(events []session.Event) (llm.Selection, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Kind != session.KindModelSelected {
			continue
		}
		var data session.ModelSelectedData
		err := json.Unmarshal(events[index].Data, &data)
		if err != nil {
			return llm.Selection{}, false
		}
		return llm.Selection{Provider: data.Provider, Model: data.Model, Thinking: data.Thinking}, true
	}
	return llm.Selection{}, false
}

func titleFromMessage(text string) string {
	text = strings.TrimSpace(text)
	const limit = 28
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

func render(writer http.ResponseWriter, request *http.Request, component templ.Component) {
	err := component.Render(request.Context(), writer)
	if err != nil {
		fmt.Printf("Web UI 渲染失败：%v\n", err)
	}
}

func renderMarkdown(source string) string {
	var output bytes.Buffer
	err := markdown.Convert([]byte(source), &output)
	if err != nil {
		return "<p>" + html.EscapeString(source) + "</p>"
	}
	return output.String()
}
