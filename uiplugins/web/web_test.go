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
	id        string
	submitted string
}

func (c *fakeConversation) SessionID() string                { return c.id }
func (*fakeConversation) State() string                      { return "idle" }
func (*fakeConversation) WaitIdle()                          {}
func (*fakeConversation) Cancel()                            {}
func (c *fakeConversation) SubmitFollowup(text string) error { c.submitted = text; return nil }
func (*fakeConversation) Steer(text string) error            { return nil }
func (*fakeConversation) InjectMemo(text string) error       { return nil }
func (*fakeConversation) SelectModel(llm.Selection) error    { return nil }
func (*fakeConversation) Book() *session.Session             { return nil }
func (*fakeConversation) Close() error                       { return nil }

type webCatalogAdapter struct{}

func (webCatalogAdapter) Name() string { return "fake" }

func (webCatalogAdapter) ProviderInfo() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name: "fake",
		Models: []llm.ModelInfo{
			{ID: "fake-fast", ThinkingLevels: []string{"off", "high"}},
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
	if !strings.Contains(body, "fake-fast") || !strings.Contains(body, "fake-pro") {
		t.Fatalf("模型菜单应来自适配器目录：%s", body)
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
	hub.Publish("s1")
	done := make(chan struct{})
	go func() { hub.Publish("s1"); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("慢客户端不该卡住账本广播")
	}
	<-updates
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

func webEvent(seq int, kind string, data any, replaces ...int) session.Event {
	raw, _ := json.Marshal(data)
	return session.Event{Seq: seq, Kind: kind, Data: raw, Replaces: replaces}
}
