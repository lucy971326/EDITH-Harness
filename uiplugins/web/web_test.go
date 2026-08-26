package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness/agents"
	"harness/chat"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

type memoryProjectStore struct {
	items map[string]projects.Project
}

func (s *memoryProjectStore) Create(project projects.Project) error {
	if _, exists := s.items[project.ID]; exists {
		return fmt.Errorf("项目已存在")
	}
	s.items[project.ID] = project
	return nil
}

func (s *memoryProjectStore) Get(id string) (projects.Project, error) {
	project, exists := s.items[id]
	if !exists {
		return projects.Project{}, fmt.Errorf("项目不存在")
	}
	return project, nil
}

func (s *memoryProjectStore) List() ([]projects.Project, error) {
	listed := make([]projects.Project, 0, len(s.items))
	for _, project := range s.items {
		listed = append(listed, project)
	}
	return listed, nil
}

func (s *memoryProjectStore) Update(project projects.Project) error {
	s.items[project.ID] = project
	return nil
}

type memoryPresetStore struct {
	items map[string][]presets.Revision
}

func (s *memoryPresetStore) Create(revision presets.Revision) error {
	if len(s.items[revision.ID]) != 0 {
		return fmt.Errorf("模式已存在")
	}
	s.items[revision.ID] = []presets.Revision{revision}
	return nil
}

func (s *memoryPresetStore) Update(revision presets.Revision) error {
	s.items[revision.ID] = append(s.items[revision.ID], revision)
	return nil
}

func (s *memoryPresetStore) Get(id string) (presets.Revision, error) {
	items := s.items[id]
	if len(items) == 0 {
		return presets.Revision{}, fmt.Errorf("模式不存在")
	}
	return items[len(items)-1], nil
}

func (s *memoryPresetStore) GetRevision(id string, number int) (presets.Revision, error) {
	items := s.items[id]
	if number < 1 || number > len(items) {
		return presets.Revision{}, fmt.Errorf("版本不存在")
	}
	return items[number-1], nil
}

func (s *memoryPresetStore) List() ([]presets.Revision, error) {
	listed := make([]presets.Revision, 0, len(s.items))
	for _, items := range s.items {
		listed = append(listed, items[len(items)-1])
	}
	return listed, nil
}

func (s *memoryPresetStore) Archive(id string) error {
	current, err := s.Get(id)
	if err != nil {
		return err
	}
	current.Revision++
	current.Archived = true
	return s.Update(current)
}

type fakeAgentService struct {
	startInput   agents.StartInput
	startCalls   int
	openCalls    int
	conversation *fakeConversation
}

func (s *fakeAgentService) StartSession(input agents.StartInput, seed ...session.Event) (agents.Conversation, error) {
	s.startCalls++
	s.startInput = input
	return s.conversation, nil
}

func (s *fakeAgentService) ResumeSession(sessionID string) (agents.Conversation, error) {
	return s.conversation, nil
}
func (s *fakeAgentService) OpenSession(sessionID string) (agents.Conversation, error) {
	s.openCalls++
	return s.conversation, nil
}
func (s *fakeAgentService) GetSession(sessionID string) (agents.Conversation, error) {
	return s.conversation, nil
}
func (s *fakeAgentService) CloseSession(sessionID string) error { return nil }
func (s *fakeAgentService) RegisterRunner(runner agents.Runner) (func(), error) {
	return func() {}, nil
}

type fakeConversation struct {
	id          string
	submitted   string
	state       string
	cancelCalls int
}

func (c *fakeConversation) SessionID() string { return c.id }
func (c *fakeConversation) State() string {
	if c.state == "" {
		return "idle"
	}
	return c.state
}
func (*fakeConversation) WaitIdle() {}
func (c *fakeConversation) Cancel() {
	c.cancelCalls++
	c.state = "idle"
}
func (c *fakeConversation) SubmitFollowup(text string) error { c.submitted = text; return nil }
func (*fakeConversation) Steer(text string) error            { return nil }
func (*fakeConversation) SelectModel(llm.Selection) error    { return nil }
func (*fakeConversation) Book() *session.Session             { return nil }
func (*fakeConversation) Close() error                       { return nil }

type webCatalogAdapter struct{}

func (webCatalogAdapter) Name() string { return "fake" }

func (webCatalogAdapter) ProviderInfo() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name: "fake",
		Models: []llm.ModelInfo{
			{ID: "fake-fast", ThinkingLevels: []string{"off", "high"}, SupportsProviderDefault: true},
			{ID: "fake-pro", ThinkingLevels: []string{"off", "max"}},
		},
	}
}

func (webCatalogAdapter) Stream(context.Context, llm.Request, func(chat.Delta)) (llm.Reply, error) {
	return llm.Reply{}, nil
}

type fakeDirectoryPicker struct {
	root  string
	err   error
	calls int
}

func (p *fakeDirectoryPicker) Pick(context.Context) (string, error) {
	p.calls++
	return p.root, p.err
}

func newWebService(t *testing.T) (*service, projects.Service, presets.Service, *session.Store, *fakeAgentService) {
	t.Helper()
	books := session.NewStore(session.NewMemoryJournal(), silentWebBroadcaster{})
	projectService := projects.New(&memoryProjectStore{items: make(map[string]projects.Project)}, books)
	presetService := presets.New(&memoryPresetStore{items: make(map[string][]presets.Revision)})
	agentService := &fakeAgentService{conversation: &fakeConversation{id: "new-session"}}
	picker := &fakeDirectoryPicker{root: t.TempDir()}
	llmService := llm.NewService()
	err := llmService.Register(webCatalogAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	return newService(projectService, presetService, books, agentService, llmService, tools.NewRegistry(), newUpdateHub(), picker), projectService, presetService, books, agentService
}

type silentWebBroadcaster struct{}

func (silentWebBroadcaster) Broadcast(string, any) {}

func TestProjectEventsHideReplacedChunksAndShowQueuedTool(t *testing.T) {
	events := []session.Event{
		webEvent(1, session.KindChunk, session.ChunkData{Delta: "半句"}),
		webEvent(2, session.KindAssistantFinal, session.AssistantFinalData{Text: "定稿"}, 1),
		webEvent(3, session.KindToolCall, session.ToolCallData{ID: "call-1", Name: "write_file"}),
		webEvent(4, session.KindToolResult, session.ToolResultData{CallID: "call-1", Status: session.ResultSuccess, Output: "完成"}),
		webEvent(5, session.KindDeliver, session.DeliverData{ID: "queued-1", Text: "第二句话", Target: "next-turn"}),
	}
	items := projectEvents(events)
	if len(items) != 3 || items[0].Text != "定稿" || items[1].ToolStatus != "已完成" || items[2].Kind != "queued" {
		t.Fatalf("账本投影不正确：%+v", items)
	}
}

func TestReadOldSessionDoesNotOpenAgent(t *testing.T) {
	service, projectService, presetService, books, agentService := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := books.Create(session.Header{ID: "old", Title: "旧会话", CreatedAt: time.Now(), ProjectID: project.ID, ProjectRoot: project.Root, PresetID: "模式", PresetRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordUserMessage("历史")
	if err != nil {
		t.Fatal(err)
	}
	err = books.Release("old")
	if err != nil {
		t.Fatal(err)
	}
	data, err := service.pageData(project.ID, "old", "", "")
	if err != nil || !data.HasSession || agentService.openCalls != 0 {
		t.Fatalf("浏览旧账不该恢复 Agent：data=%+v err=%v open=%d", data.Header, err, agentService.openCalls)
	}
}

func TestFirstMessageStartsSessionAndRemembersPreset(t *testing.T) {
	service, projectService, presetService, _, agentService := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "极简"})
	if err != nil {
		t.Fatal(err)
	}
	selection := "fake\x1ffake-fast\x1foff"
	request := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader("project_id="+project.ID+"&preset_id=%E6%9E%81%E7%AE%80&model_selection="+url.QueryEscape(selection)+"&text=%E4%BD%A0%E5%A5%BD"))
	request.Header.Set("HX-Request", "true")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleSubmitMessage(writer, request)
	if writer.Code != http.StatusNoContent || agentService.startCalls != 1 || agentService.conversation.submitted != "你好" {
		t.Fatalf("首次发送没有启动并投递：code=%d calls=%d text=%q", writer.Code, agentService.startCalls, agentService.conversation.submitted)
	}
	stored, err := projectService.Get(project.ID)
	if err != nil || stored.LastPresetID != "极简" {
		t.Fatalf("首次发送没有记住模式：%+v err=%v", stored, err)
	}
}

func TestPickProjectUsesDirectoryName(t *testing.T) {
	service, projectService, _, _, _ := newWebService(t)
	picker, ok := service.picker.(*fakeDirectoryPicker)
	if !ok {
		t.Fatal("测试应使用假目录选择器")
	}
	request := httptest.NewRequest(http.MethodPost, "/projects/pick", nil)
	writer := httptest.NewRecorder()
	service.handlePickProject(writer, request)
	if writer.Code != http.StatusSeeOther || picker.calls != 1 {
		t.Fatalf("选择目录后应跳转：code=%d calls=%d", writer.Code, picker.calls)
	}
	listed, err := projectService.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("项目没有登记：%+v err=%v", listed, err)
	}
	wantRoot, err := filepath.EvalSymlinks(picker.root)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Name != filepath.Base(wantRoot) || listed[0].Root != wantRoot {
		t.Fatalf("项目应直接来自所选目录：%+v", listed[0])
	}
}

func TestPickProjectDoesNothingWhenCancelled(t *testing.T) {
	service, projectService, _, _, _ := newWebService(t)
	service.picker = &fakeDirectoryPicker{err: errDirectoryPickCancelled}
	request := httptest.NewRequest(http.MethodPost, "/projects/pick", nil)
	writer := httptest.NewRecorder()
	service.handlePickProject(writer, request)
	if writer.Code != http.StatusSeeOther {
		t.Fatalf("取消选择应返回首页：code=%d", writer.Code)
	}
	listed, err := projectService.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("取消选择不应登记项目：%+v err=%v", listed, err)
	}
}

func TestChatModelPickerComesFromProviderCatalog(t *testing.T) {
	service, projectService, presetService, _, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := service.pageData(project.ID, "", "1", "")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	err = ChatPanel(data).Render(context.Background(), writer)
	if err != nil {
		t.Fatal(err)
	}
	body := writer.Body.String()
	if !strings.Contains(body, `class="model-picker-group-title">fake</div>`) || !strings.Contains(body, "fake-fast") || !strings.Contains(body, "fake-pro") {
		t.Fatalf("模型菜单应来自适配器目录且按分组展示：%s", body)
	}
	if !strings.Contains(body, `data-thinking-option`) || !strings.Contains(body, ">Off<") || !strings.Contains(body, ">High<") {
		t.Fatalf("思考档位应来自当前模型能力：%s", body)
	}
	if !strings.Contains(body, ">Default<") {
		t.Fatalf("支持服务商默认的模型应展示 Default：%s", body)
	}
	if strings.Contains(body, `data-thinking="max"`) {
		t.Fatalf("当前 fake-fast 不应展示 fake-pro 专属的 max 档位：%s", body)
	}
}

func TestSessionHeaderKeepsModeAndReservesTrajectoryTab(t *testing.T) {
	service, projectService, presetService, books, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = books.Create(session.Header{
		ID:             "session-ui",
		Title:          "一个会话",
		CreatedAt:      time.Now(),
		ProjectID:      project.ID,
		ProjectRoot:    project.Root,
		PresetID:       "模式",
		PresetRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = books.Release("session-ui")
	if err != nil {
		t.Fatal(err)
	}

	data, err := service.pageData(project.ID, "session-ui", "", "")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	err = ChatPanel(data).Render(context.Background(), writer)
	if err != nil {
		t.Fatal(err)
	}
	body := writer.Body.String()
	if !strings.Contains(body, `class="chat-mode"`) || !strings.Contains(body, ">模式</span>") {
		t.Fatalf("会话标题区应展示当前模式：%s", body)
	}
	if !strings.Contains(body, ">对话</button>") || !strings.Contains(body, ">轨迹</button>") {
		t.Fatalf("会话标题区应预留对话和轨迹入口：%s", body)
	}
	if strings.Contains(body, "locked-preset") || strings.Contains(body, "已锁定") || strings.Contains(body, "Session log") {
		t.Fatalf("会话页不应重复展示锁定文案或 Session log：%s", body)
	}
}

func TestThinkingChoicesFollowSelectedModel(t *testing.T) {
	service, projectService, presetService, _, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := service.pageData(project.ID, "", "1", "")
	if err != nil {
		t.Fatal(err)
	}
	choices := thinkingChoices(data.Providers, llm.Selection{Provider: "fake", Model: "fake-pro"})
	if len(choices) != 2 || choices[0].Value != "off" || choices[1].Value != "max" {
		t.Fatalf("fake-pro 的档位必须独立于 fake-fast：%+v", choices)
	}
}

func TestSelectProviderDefaultStoresEmptyThinking(t *testing.T) {
	service, projectService, _, _, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/sessions/model", strings.NewReader("project_id="+project.ID+"&draft=1&model_id="+url.QueryEscape("fake\x1ffake-fast")+"&thinking="))
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "model-picker")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleSelectModel(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("选择 Default 失败，实际状态 %d：%s", writer.Code, writer.Body.String())
	}
	stored, err := projectService.Get(project.ID)
	if err != nil || stored.LastModel.Thinking != "" {
		t.Fatalf("Default 应以空 thinking 记住：%+v err=%v", stored.LastModel, err)
	}
}

func TestFirstMessageWithSplitModelAndThinking(t *testing.T) {
	service, projectService, presetService, _, agentService := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "极简"})
	if err != nil {
		t.Fatal(err)
	}
	modelID := "fake\x1ffake-pro"
	request := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader("project_id="+project.ID+"&preset_id=%E6%9E%81%E7%AE%80&model_id="+url.QueryEscape(modelID)+"&thinking=max&text=%E4%BD%A0%E5%A5%BD"))
	request.Header.Set("HX-Request", "true")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleSubmitMessage(writer, request)
	if writer.Code != http.StatusNoContent || agentService.startCalls != 1 || agentService.conversation.submitted != "你好" {
		t.Fatalf("首次发送没有启动并投递：code=%d calls=%d text=%q", writer.Code, agentService.startCalls, agentService.conversation.submitted)
	}
	if agentService.startInput.Model.Provider != "fake" || agentService.startInput.Model.Model != "fake-pro" || agentService.startInput.Model.Thinking != "max" {
		t.Fatalf("传递给会话的模型不正确：%+v", agentService.startInput.Model)
	}
	stored, err := projectService.Get(project.ID)
	if err != nil || stored.LastModel.Model != "fake-pro" || stored.LastModel.Thinking != "max" {
		t.Fatalf("首次发送没有记住模型：%+v err=%v", stored, err)
	}
}

func TestSelectModelAutoResolvesIncompatibleThinking(t *testing.T) {
	service, projectService, _, _, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// fake-fast 只支持 "off", "high"；传入 "max" 应自动降级为 "off"（首个档位）
	request := httptest.NewRequest(http.MethodPost, "/sessions/model", strings.NewReader("project_id="+project.ID+"&draft=1&model_id="+url.QueryEscape("fake\x1ffake-fast")+"&thinking=max"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleSelectModel(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("切换草稿模型失败，实际状态 %d：%s", writer.Code, writer.Body.String())
	}
	stored, err := projectService.Get(project.ID)
	if err != nil || stored.LastModel.Model != "fake-fast" || stored.LastModel.Thinking != "off" {
		t.Fatalf("不兼容的思考档位应自动校准为支持的档位：%+v err=%v", stored.LastModel, err)
	}
}

func TestSelectThinkingOnlyPreservesModel(t *testing.T) {
	service, projectService, _, _, _ := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 先设置基础模型为 fake-fast
	err = projectService.RememberModel(project.ID, llm.Selection{Provider: "fake", Model: "fake-fast", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	// 切换档位为 high（同时带 model_id 或仅带 thinking）
	request := httptest.NewRequest(http.MethodPost, "/sessions/model", strings.NewReader("project_id="+project.ID+"&draft=1&model_id="+url.QueryEscape("fake\x1ffake-fast")+"&thinking=high"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleSelectModel(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("切换思考档位失败，实际状态 %d：%s", writer.Code, writer.Body.String())
	}
	stored, err := projectService.Get(project.ID)
	if err != nil || stored.LastModel.Model != "fake-fast" || stored.LastModel.Thinking != "high" {
		t.Fatalf("思考档位应成功更新为 high：%+v err=%v", stored.LastModel, err)
	}
}

func TestUpdatePresetLetsPresetServiceCreateNextRevision(t *testing.T) {
	service, _, presetService, _, _ := newWebService(t)
	err := presetService.Create(presets.Preset{ID: "猫娘", SystemPrompt: "旧提示词"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/presets/update", strings.NewReader("id=%E7%8C%AB%E5%A8%98&system_prompt=%E6%96%B0%E6%8F%90%E7%A4%BA%E8%AF%8D"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleUpdatePreset(writer, request)
	if writer.Code != http.StatusSeeOther {
		t.Fatalf("编辑模式应跳转回列表，实际 %d：%s", writer.Code, writer.Body.String())
	}
	updated, err := presetService.Get("猫娘")
	if err != nil || updated.Revision != 2 || updated.SystemPrompt != "新提示词" {
		t.Fatalf("编辑应保存为第 2 版：%+v err=%v", updated, err)
	}
}

func TestSlowUpdateSubscriberDoesNotBlockPublish(t *testing.T) {
	hub := newUpdateHub()
	updates, unregister := hub.Subscribe("s1")
	defer unregister()
	hub.Publish("s1", updateNotice{Chat: true})
	done := make(chan struct{})
	go func() { hub.Publish("s1", updateNotice{Chat: true}); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("慢客户端不该卡住账本广播")
	}
	<-updates
}

func TestUpdateNoticeMergesChatAndComposerRefreshes(t *testing.T) {
	hub := newUpdateHub()
	updates, unregister := hub.Subscribe("s1")
	defer unregister()

	hub.Publish("s1", updateNotice{Chat: true})
	hub.Publish("s1", updateNotice{Composer: true})
	notice := <-updates
	if !notice.Chat || !notice.Composer {
		t.Fatalf("合并通知不能丢掉输入区刷新：%+v", notice)
	}
}

func TestCrossOriginWriteRejected(t *testing.T) {
	service, _, _, _, _ := newWebService(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/projects/pick", nil)
	request.Host = "127.0.0.1:8080"
	request.Header.Set("Origin", "http://evil.example")
	writer := httptest.NewRecorder()
	service.routes().ServeHTTP(writer, request)
	if writer.Code != http.StatusForbidden {
		t.Fatalf("跨源写请求应拒绝，实际 %d", writer.Code)
	}
}

func TestRenderMarkdownUsesGFMAndEscapesRawHTML(t *testing.T) {
	rendered := renderMarkdown("# 标题\n\n- 条目\n\n| 名称 | 值 |\n| --- | --- |\n| alpha | beta |\n\n<script>alert(1)</script>")
	if !strings.Contains(rendered, "<h1>标题</h1>") || !strings.Contains(rendered, "<table>") {
		t.Fatalf("Markdown 应支持标题和 GFM 表格：%s", rendered)
	}
	if strings.Contains(rendered, "<script>") || !strings.Contains(rendered, "raw HTML omitted") {
		t.Fatalf("Markdown 不应直接输出原始 HTML：%s", rendered)
	}
}

func TestBusyComposerUsesOneCancelButton(t *testing.T) {
	data := PageData{
		Project:        projects.Project{ID: "project-1"},
		Header:         session.Header{ID: "session-1"},
		SelectedPreset: presets.Preset{ID: "模式"},
		SelectedModel:  llm.Selection{Provider: "fake", Model: "fake-fast"},
		HasProject:     true,
		HasSession:     true,
		HasPreset:      true,
		Busy:           true,
	}
	writer := httptest.NewRecorder()
	err := MessageBox(data).Render(context.Background(), writer)
	if err != nil {
		t.Fatal(err)
	}
	body := writer.Body.String()
	if !strings.Contains(body, `id="composer"`) || !strings.Contains(body, `class="send-button is-cancel"`) || !strings.Contains(body, `aria-label="取消"`) {
		t.Fatalf("忙碌时应显示合并后的取消按钮：%s", body)
	}
	if !strings.Contains(body, `hx-target="#composer"`) {
		t.Fatalf("取消请求只能刷新输入区：%s", body)
	}
	if strings.Contains(body, `type="submit"`) || strings.Contains(body, "cancel-button") {
		t.Fatalf("忙碌时不应同时存在发送按钮或独立取消按钮：%s", body)
	}
}

func TestCancelRouteStopsConversationAndRendersSendButton(t *testing.T) {
	service, projectService, presetService, books, agentService := newWebService(t)
	project, err := projectService.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{ID: "模式"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = books.Create(session.Header{
		ID:             "session-cancel",
		Title:          "取消测试",
		CreatedAt:      time.Now(),
		ProjectID:      project.ID,
		ProjectRoot:    project.Root,
		PresetID:       "模式",
		PresetRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	agentService.conversation.state = "busy"
	body := "project_id=" + url.QueryEscape(project.ID) + "&session_id=session-cancel"
	request := httptest.NewRequest(http.MethodPost, "/sessions/cancel", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	service.handleCancelSession(writer, request)
	if agentService.conversation.cancelCalls != 1 {
		t.Fatalf("取消请求应调用会话取消：calls=%d", agentService.conversation.cancelCalls)
	}
	if strings.Contains(writer.Body.String(), `class="send-button is-cancel"`) {
		t.Fatalf("取消完成后不应继续显示取消按钮：%s", writer.Body.String())
	}
	if !strings.Contains(writer.Body.String(), `id="composer"`) || strings.Contains(writer.Body.String(), `id="chat-panel"`) {
		t.Fatalf("取消响应只能返回输入区：%s", writer.Body.String())
	}
}

func webEvent(seq int, kind string, data any, replaces ...int) session.Event {
	raw, _ := json.Marshal(data)
	return session.Event{Seq: seq, Kind: kind, Data: raw, Replaces: replaces}
}
