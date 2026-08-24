package terminal

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/loop"
	"harness/persistence/jsonl"
	"harness/persistence/profilejson"
	"harness/session"
	"harness/tools"
)

type echoAdapter struct{}

type tuiConversationStub struct{}

func (tuiConversationStub) AgentID() string             { return "test" }
func (tuiConversationStub) SessionID() string           { return "test-session" }
func (tuiConversationStub) State() string               { return "idle" }
func (tuiConversationStub) WaitIdle()                   {}
func (tuiConversationStub) Cancel()                     {}
func (tuiConversationStub) SubmitFollowup(string) error { return nil }
func (tuiConversationStub) Steer(string) error          { return nil }
func (tuiConversationStub) InjectMemo(string) error     { return nil }
func (tuiConversationStub) Book() *session.Session      { return nil }
func (tuiConversationStub) Close() error                { return nil }

func (echoAdapter) Name() string {
	return "echo"
}

func (echoAdapter) ProviderInfo() llm.ProviderInfo {
	return llm.ProviderInfo{Name: "echo", ThinkingLevels: []string{"off"}}
}

func (echoAdapter) Stream(_ context.Context, _ llm.Request, onDelta func(chat.Delta)) (llm.Reply, error) {
	if onDelta != nil {
		onDelta(chat.Delta{Text: "收到"})
	}
	return llm.Reply{Text: "收到", StopReason: "stop"}, nil
}

func TestTUIMessageFlowPrintsFinalOnlyOnce(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(
		profilejson.Plugin{Root: t.TempDir()},
		jsonl.Plugin{Root: t.TempDir()},
		session.Plugin{},
		llm.Plugin{},
		tools.Plugin{},
		agents.Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	models, err := llm.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = models.Register(echoAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	err = app.Install(loop.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	roster, err := agents.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = roster.CreateAgent(agents.AgentProfile{
		ID:       "小红",
		Provider: "echo",
		Model:    "echo-model",
		Thinking: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := roster.StartSession("小红", "第一段")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conversation.Close()
	})
	model := newTUIModel(roster, models, tools.NewRegistry(), newEventBuffer())
	model.conversation = conversation
	model.focus = focusChat
	model.textarea.SetValue("你好")
	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil {
		t.Fatal("Enter 应该生成发送命令")
	}
	message := command()
	_, _ = model.Update(message)
	conversation.WaitIdle()
	model.transcript = conversation.Book().Events()
	text := renderTranscript(model.transcript)
	if strings.Count(text, "收到") != 1 {
		t.Fatalf("流式回复定稿不该重复显示：\n%s", text)
	}
}

func TestHistoryHidesReplacedChunksAndThinking(t *testing.T) {
	events := []session.Event{
		{Kind: session.KindChunk, Seq: 1, Data: []byte(`{"Delta":"回"}`)},
		{Kind: session.KindChunk, Seq: 2, Data: []byte(`{"Delta":"答"}`)},
		{Kind: session.KindAssistantFinal, Seq: 3, Data: []byte(`{"Text":"回答","Thinking":"不能显示"}`), Replaces: []int{1, 2}},
	}
	text := renderTranscript(events)
	if strings.Count(text, "回答") != 1 || strings.Contains(text, "不能显示") {
		t.Fatalf("历史投影不对：%s", text)
	}
}

func TestPreviewDoesNotCutChineseRune(t *testing.T) {
	text := strings.Repeat("你", 241)
	got := preview(text)
	if !utf8.ValidString(got) {
		t.Fatalf("预览切坏了 UTF-8：%q", got)
	}
}

func TestUserMessageAppearsInTranscriptProjection(t *testing.T) {
	text := renderTranscript([]session.Event{{
		Kind: session.KindUserMessage,
		Seq:  1,
		Data: []byte(`{"Text":"你好"}`),
	}})
	if !strings.Contains(text, "你好") {
		t.Fatalf("用户消息应该出现在历史投影中：%q", text)
	}
}

func TestArchivedAgentCannotCreateSession(t *testing.T) {
	model := newTUIModel(nil, nil, tools.NewRegistry(), newEventBuffer())
	model.profiles = []agents.AgentProfile{{ID: "旧小红", Archived: true}}
	model.focus = focusSessions

	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command != nil {
		t.Fatal("归档 Agent 不应该启动新会话命令")
	}
	if model.problem != "归档 Agent 不能新建会话" {
		t.Fatalf("归档 Agent 的提示不对：%q", model.problem)
	}
}

func TestChatPrintableKeysReachTextarea(t *testing.T) {
	model := newTUIModel(nil, nil, tools.NewRegistry(), newEventBuffer())
	model.focus = focusChat
	model.conversation = tuiConversationStub{}
	model.textarea.Focus()
	_, command := model.Update(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	if command != nil {
		t.Fatal("普通字符不应该触发异步命令")
	}
	if model.textarea.Value() != "a" {
		t.Fatalf("普通字符没有进入输入框：%q", model.textarea.Value())
	}
}

func TestNewSessionOpensAVisibleForm(t *testing.T) {
	model := newTUIModel(nil, nil, tools.NewRegistry(), newEventBuffer())
	model.profiles = []agents.AgentProfile{{ID: "小红"}}
	model.focus = focusSessions

	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil {
		t.Fatal("新建会话应该把输入框聚焦")
	}
	if !model.sessionForm {
		t.Fatal("新建会话应该打开表单")
	}
	if !strings.Contains(renderSessionForm(model), "新建会话") {
		t.Fatal("新建会话表单必须有标题")
	}
}

func TestNewAgentFormShowsEachFieldLabelOnce(t *testing.T) {
	model := newTUIModel(nil, &llm.Service{}, tools.NewRegistry(), newEventBuffer())
	model.form = &agentForm{
		inputs:    []textinput.Model{newFormInput("名字"), newFormInput("模型"), newFormInput("人设"), newFormInput("工具")},
		providers: []llm.ProviderInfo{{Name: "deepseek", ThinkingLevels: []string{"off"}}},
	}

	view := renderForm(model)
	if strings.Count(view, "Agent 名字") != 1 {
		t.Fatalf("字段名不该重复：%q", view)
	}
}
