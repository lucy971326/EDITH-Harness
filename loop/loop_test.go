package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Roster 是测试用门面，让旧测试通过公开 agents.Service 进入系统。
type Roster = testRoster
type ProfileStore = agents.ProfileStore
type AgentProfile = agents.AgentProfile

type testRoster struct {
	agents.Service
	toolsReg *tools.Registry
}

func cloneProfile(profile AgentProfile) AgentProfile {
	copied := profile
	copied.Tools = append([]string(nil), profile.Tools...)
	return copied
}

// silentBroadcaster 给账本一个不出声的广播口。
type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

// memoryJournalPlugin 给 loop 测试装内存账本，避免测试碰本地文件。
type memoryJournalPlugin struct {
	journal session.Journal
}

// memoryProfilePlugin 给 loop 测试装内存 Agent 档案库。
type memoryProfilePlugin struct {
	store ProfileStore
}

func (memoryProfilePlugin) Name() string { return "test-memory-profiles" }

func (p memoryProfilePlugin) Start(app *core.App) error {
	app.RegisterService("agent-profiles", p.store)
	return nil
}

// blockingProfileStore 让更新停在真正写档案的位置，专门测名册锁会不会被中途放开。
type blockingProfileStore struct {
	base    *memoryProfileStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingProfileStore) Create(profile AgentProfile) error {
	return s.base.Create(profile)
}

func (s *blockingProfileStore) Get(id string) (AgentProfile, error) {
	return s.base.Get(id)
}

func (s *blockingProfileStore) List() ([]AgentProfile, error) {
	return s.base.List()
}

func (s *blockingProfileStore) Archive(id string) error {
	return s.base.Archive(id)
}

func (s *blockingProfileStore) Update(profile AgentProfile) error {
	close(s.entered)
	<-s.release
	return s.base.Update(profile)
}

// memoryProfileStore 是测试专用的档案库，语义与真正持久化档案库一致。
type memoryProfileStore struct {
	mu       sync.Mutex
	profiles map[string]AgentProfile
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{profiles: make(map[string]AgentProfile)}
}

func (s *memoryProfileStore) Create(profile AgentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.profiles[profile.ID]; exists {
		return fmt.Errorf("agent %s 已存在", profile.ID)
	}
	s.profiles[profile.ID] = cloneProfile(profile)
	return nil
}

func (s *memoryProfileStore) Update(profile AgentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, exists := s.profiles[profile.ID]
	if !exists {
		return fmt.Errorf("agent %s 不存在", profile.ID)
	}
	if profile.Revision != old.Revision+1 {
		return fmt.Errorf("agent %s 版本不连续", profile.ID)
	}
	s.profiles[profile.ID] = cloneProfile(profile)
	return nil
}

func (s *memoryProfileStore) Get(id string) (AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, exists := s.profiles[id]
	if !exists {
		return AgentProfile{}, fmt.Errorf("agent %s 不存在", id)
	}
	return cloneProfile(profile), nil
}

func (s *memoryProfileStore) List() ([]AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles := make([]AgentProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		profiles = append(profiles, cloneProfile(profile))
	}
	return profiles, nil
}

func (s *memoryProfileStore) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, exists := s.profiles[id]
	if !exists {
		return fmt.Errorf("agent %s 不存在", id)
	}
	profile.Archived = true
	profile.Revision++
	s.profiles[id] = profile
	return nil
}

func (memoryJournalPlugin) Name() string { return "test-memory-journal" }

func (p memoryJournalPlugin) Start(app *core.App) error {
	app.RegisterService("journal", p.journal)
	return nil
}

// scriptedAdapter 是剧本适配器：每次问模型按剧本回话，顺便记下收到的每个请求。
type scriptedAdapter struct {
	mu       sync.Mutex
	scripts  []scriptCall // 第 N 次请求用第 N 个剧本，超了报错
	requests []llm.Request
	block    chan struct{} // 非空时第一问会卡在这，用来制造"忙"
}

type scriptCall struct {
	deltas []string
	reply  llm.Reply
	hang   bool // true = 吐完字后挂住，等取消（测打断）
}

func (a *scriptedAdapter) Name() string {
	return "剧本"
}

func (a *scriptedAdapter) Stream(ctx context.Context, req llm.Request, onDelta func(chat.Delta)) (llm.Reply, error) {
	a.mu.Lock()
	index := len(a.requests)
	if index >= len(a.scripts) {
		a.mu.Unlock()
		return llm.Reply{}, llm.NewError("剧本", "bad_request", "剧本用完了")
	}
	script := a.scripts[index]
	a.requests = append(a.requests, req)
	block := a.block
	a.mu.Unlock()

	if block != nil && index == 0 {
		select {
		case <-block:
		case <-ctx.Done():
			return llm.Reply{}, llm.NewError("剧本", "cancelled", "被取消")
		}
	}

	text := ""
	for _, delta := range script.deltas {
		if onDelta != nil {
			onDelta(chat.Delta{Text: delta})
		}
		text += delta
	}

	if script.hang {
		select {
		case <-ctx.Done():
			return llm.Reply{}, llm.NewError("剧本", "cancelled", "用户打断了")
		case <-time.After(30 * time.Second):
		}
	}

	script.reply.Text = text
	return script.reply, nil
}

func (a *scriptedAdapter) sawRequests() []llm.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]llm.Request(nil), a.requests...)
}

// newTestStack 搭一套最小可跑的家当：core 场地 + 账本 + 插座排（剧本适配器）+ 工具 + loop。
func newTestStack(t *testing.T, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	return newTestStackWithStores(t, session.NewMemoryJournal(), newMemoryProfileStore(), scripts...)
}

func newTestStackWithJournal(t *testing.T, journal session.Journal, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	return newTestStackWithStores(t, journal, newMemoryProfileStore(), scripts...)
}

func newTestStackWithStores(t *testing.T, journal session.Journal, profiles ProfileStore, scripts ...scriptCall) (*core.App, *Roster, *scriptedAdapter, *tools.Registry) {
	t.Helper()

	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(
		memoryProfilePlugin{store: profiles},
		memoryJournalPlugin{journal: journal},
		session.Plugin{},
		llm.Plugin{},
		tools.Plugin{},
	)
	if err != nil {
		t.Fatalf("装基础插件失败：%v", err)
	}

	adapter := &scriptedAdapter{scripts: scripts}
	llmSvc, err := llm.Get(app)
	if err != nil {
		t.Fatalf("取模型入口失败：%v", err)
	}
	err = llmSvc.Register(adapter)
	if err != nil {
		t.Fatalf("插适配器失败：%v", err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		t.Fatalf("取工具登记处失败：%v", err)
	}

	err = app.Install(agents.Plugin{}, Plugin{})
	if err != nil {
		t.Fatalf("装 loop 插件失败：%v", err)
	}
	service, err := agents.Get(app)
	if err != nil {
		t.Fatalf("取 Agent 管理入口失败：%v", err)
	}
	roster := &testRoster{Service: service, toolsReg: registry}

	return app, roster, adapter, registry
}

func startTestSession(t *testing.T, roster *Roster, id string, config AgentConfig, seed ...session.Event) *Conversation {
	t.Helper()
	provider := config.Provider
	if provider == "" {
		provider = "剧本"
	}
	model := config.Model
	if model == "" {
		model = "test-model"
	}
	thinking := config.Thinking
	if thinking == "" {
		thinking = "off"
	}
	profile := AgentProfile{ID: id, Provider: provider, Model: model, Thinking: thinking, SystemPrompt: config.SystemPrompt, Tools: roster.toolsReg.Names()}
	err := roster.CreateAgent(profile)
	if err != nil {
		t.Fatalf("建 agent 档案失败：%v", err)
	}
	conversation, err := roster.StartSession(id, id, seed...)
	if err != nil {
		t.Fatalf("开会话失败：%v", err)
	}
	return conversation.(*Conversation)
}

func TestRosterResumesExistingAgent(t *testing.T) {
	journal := session.NewMemoryJournal()
	profiles := newMemoryProfileStore()
	firstApp, firstRoster, _, _ := newTestStackWithStores(t, journal, profiles)
	first := startTestSession(t, firstRoster, "小刚", AgentConfig{})
	_, err := first.Book().RecordUserMessage("明天继续")
	if err != nil {
		t.Fatal(err)
	}
	firstApp.Close()

	_, secondRoster, _, _ := newTestStackWithStores(t, journal, profiles)
	resumed, err := secondRoster.ResumeSession("小刚")
	if err != nil {
		t.Fatal(err)
	}
	history := resumed.Book().ModelHistory()
	if len(history) != 1 || history[0].Text != "明天继续" {
		t.Fatalf("恢复后应该读回旧会话：%+v", history)
	}
}

func TestRecoveryBroadcastDoesNotHoldRosterLock(t *testing.T) {
	app, roster, _, _ := newTestStack(t)
	seen := make(chan error, 1)
	app.Subscribe(session.EventAppended, func(payload any) {
		appended := payload.(session.Appended)
		if appended.SessionID != "恢复会话" || appended.Event.Kind != session.KindToolResult {
			return
		}
		_, err := roster.GetSession("恢复会话")
		seen <- err
	})
	err := roster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "m", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	seed := []session.Event{
		event(session.KindToolCall, 1, session.ToolCallData{ID: "c1", Name: "mail"}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := roster.StartSession("小红", "恢复会话", seed...)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("恢复补账时不该被广播观察者卡死")
	}
	select {
	case err := <-seen:
		if err == nil {
			t.Fatal("会话准备好前不该把半成品交给观察者")
		}
	case <-time.After(time.Second):
		t.Fatal("恢复补账应该广播给观察者")
	}
}

func TestUpdateAndStartSessionCannotMixTwoProfileVersions(t *testing.T) {
	profiles := &blockingProfileStore{
		base:    newMemoryProfileStore(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	_, roster, adapter, _ := newTestStackWithStores(t, session.NewMemoryJournal(), profiles,
		scriptCall{deltas: []string{"好"}, reply: llm.Reply{StopReason: "stop"}},
	)
	err := roster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "old-model", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	updated := make(chan error, 1)
	go func() {
		updated <- roster.UpdateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "new-model", Thinking: "off"})
	}()
	<-profiles.entered
	started := make(chan agents.Conversation, 1)
	startErr := make(chan error, 1)
	go func() {
		conversation, err := roster.StartSession("小红", "新会话")
		if err != nil {
			startErr <- err
			return
		}
		started <- conversation
	}()
	select {
	case <-started:
		t.Fatal("档案更新尚未写完时，不该开出会话")
	case err := <-startErr:
		t.Fatalf("开会话不该抢在更新前失败：%v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(profiles.release)
	err = <-updated
	if err != nil {
		t.Fatal(err)
	}
	conversation := <-started
	err = conversation.SubmitFollowup("你好")
	if err != nil {
		t.Fatal(err)
	}
	conversation.WaitIdle()
	if adapter.sawRequests()[0].Model != "new-model" {
		t.Fatalf("会话该只看到写完后的新档案：%+v", adapter.sawRequests()[0])
	}
}

func TestOneAgentCanRunTwoSessions(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"会话一"}, reply: llm.Reply{StopReason: "stop"}},
		scriptCall{deltas: []string{"会话二"}, reply: llm.Reply{StopReason: "stop"}},
	)
	err := roster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "m", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := roster.StartSession("小红", "张三")
	if err != nil {
		t.Fatal(err)
	}
	second, err := roster.StartSession("小红", "李四")
	if err != nil {
		t.Fatal(err)
	}
	if first.AgentID() != "小红" || second.AgentID() != "小红" || first.SessionID() == second.SessionID() {
		t.Fatalf("Agent 和会话身份没拆开：%s/%s, %s/%s", first.AgentID(), first.SessionID(), second.AgentID(), second.SessionID())
	}
	if first.Book().AgentID() != "小红" || first.Book().ProfileRevision() != 1 {
		t.Fatalf("会话封面没记住 Agent 归属：%s / v%d", first.Book().AgentID(), first.Book().ProfileRevision())
	}
	err = first.SubmitFollowup("给张三")
	if err != nil {
		t.Fatal(err)
	}
	err = second.SubmitFollowup("给李四")
	if err != nil {
		t.Fatal(err)
	}
	first.WaitIdle()
	second.WaitIdle()
	if first.Book().ModelHistory()[0].Text != "给张三" || second.Book().ModelHistory()[0].Text != "给李四" {
		t.Fatalf("两段会话串台了：%+v / %+v", first.Book().ModelHistory(), second.Book().ModelHistory())
	}
}

func TestArchiveBlocksNewSessionButAllowsResume(t *testing.T) {
	_, roster, _, _ := newTestStack(t)
	err := roster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "m", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := roster.StartSession("小红", "旧会话")
	if err != nil {
		t.Fatal(err)
	}
	err = conversation.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = roster.ArchiveAgent("小红")
	if err != nil {
		t.Fatal(err)
	}
	_, err = roster.StartSession("小红", "新会话")
	if err == nil {
		t.Fatal("归档 Agent 不该再开新会话")
	}
	resumed, err := roster.ResumeSession("旧会话")
	if err != nil {
		t.Fatalf("归档 Agent 的旧会话仍该能恢复：%v", err)
	}
	if resumed.AgentID() != "小红" {
		t.Fatalf("恢复后 Agent 不对：%s", resumed.AgentID())
	}
}

func TestClosingOneSessionKeepsAgentToolsForTheOther(t *testing.T) {
	_, roster, _, registry := newTestStack(t)
	echoTool(registry, t)
	err := registry.Register(tools.Tool{Schema: chat.ToolSchema{Name: "hidden"}})
	if err != nil {
		t.Fatal(err)
	}
	err = roster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "m", Thinking: "off", Tools: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := roster.StartSession("小红", "会话一")
	if err != nil {
		t.Fatal(err)
	}
	second, err := roster.StartSession("小红", "会话二")
	if err != nil {
		t.Fatal(err)
	}
	_, visible := registry.Lookup("hidden", "小红")
	if visible {
		t.Fatal("白名单外工具不该给 Agent 看见")
	}
	err = first.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, visible = registry.Lookup("hidden", "小红")
	if visible {
		t.Fatal("关一段会话不该收掉另一段的 Agent 工具表")
	}
	err = second.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, visible = registry.Lookup("hidden", "小红")
	if !visible {
		t.Fatal("最后一段会话关闭后，Agent 小表该收掉")
	}
}

func TestResumeUsesCurrentAgentProfile(t *testing.T) {
	journal := session.NewMemoryJournal()
	profiles := newMemoryProfileStore()
	firstApp, firstRoster, _, _ := newTestStackWithStores(t, journal, profiles,
		scriptCall{deltas: []string{"旧回复"}, reply: llm.Reply{StopReason: "stop"}},
	)
	err := firstRoster.CreateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "old-model", Thinking: "off", SystemPrompt: "旧人设"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRoster.StartSession("小红", "旧会话")
	if err != nil {
		t.Fatal(err)
	}
	err = first.SubmitFollowup("你好")
	if err != nil {
		t.Fatal(err)
	}
	first.WaitIdle()
	firstApp.Close()

	_, secondRoster, adapter, _ := newTestStackWithStores(t, journal, profiles,
		scriptCall{deltas: []string{"新回复"}, reply: llm.Reply{StopReason: "stop"}},
	)
	err = secondRoster.UpdateAgent(AgentProfile{ID: "小红", Provider: "剧本", Model: "new-model", Thinking: "off", SystemPrompt: "新人设"})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := secondRoster.ResumeSession("旧会话")
	if err != nil {
		t.Fatal(err)
	}
	err = resumed.SubmitFollowup("继续")
	if err != nil {
		t.Fatal(err)
	}
	resumed.WaitIdle()
	request := adapter.sawRequests()[0]
	if request.Model != "new-model" || request.Messages[0].Text != "新人设" {
		t.Fatalf("恢复后没有使用当前档案：%+v", request)
	}
}

// echoTool 一个回显工具，给工具闭环用。
func echoTool(registry *tools.Registry, t *testing.T) {
	err := registry.Register(tools.Tool{
		Schema: chat.ToolSchema{Name: "echo", Description: "回显", Parameters: []byte(`{"type":"object"}`)},
		Execute: func(ctx context.Context, arguments json.RawMessage) (string, error) {
			var args struct {
				Text string
			}
			err := json.Unmarshal(arguments, &args)
			if err != nil {
				return "", err
			}
			return args.Text, nil
		},
	})
	if err != nil {
		t.Fatalf("登记工具失败：%v", err)
	}
}

func kinds(events []session.Event) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.Kind
	}
	return out
}

func TestHappyTurnFullLedger(t *testing.T) {
	_, roster, _, registry := newTestStack(t,
		scriptCall{deltas: []string{"好", "的"}, reply: llm.Reply{StopReason: "stop"}},
	)
	echoTool(registry, t)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "test-model", SystemPrompt: "你是测试员"})

	err := agent.SubmitFollowup("你好")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	events := agent.Book().Events()
	got := kinds(events)
	want := []string{
		session.KindDeliver, session.KindClaim, session.KindUserMessage,
		session.KindTurnStart, session.KindStepStart, session.KindSnapshot,
		session.KindChunk, session.KindChunk, session.KindAssistantFinal,
		session.KindStepEnd, session.KindTurnEnd,
	}
	if len(got) != len(want) {
		t.Fatalf("一轮的账不对：\ngot  %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("一轮的账不对：\ngot  %v\nwant %v", got, want)
		}
	}

	if agent.State() != "idle" {
		t.Fatalf("干完该闲：got %s", agent.State())
	}

	history := agent.Book().ModelHistory()
	if len(history) != 2 {
		t.Fatalf("历史该是 user+assistant 两条：got %v", history)
	}
	if history[0].Text != "你好" || history[1].Text != "好的" {
		t.Fatalf("历史内容不对：%+v", history)
	}
}

func TestToolLoopRunsSecondStep(t *testing.T) {
	_, roster, adapter, registry := newTestStack(t,
		scriptCall{deltas: []string{"我查查"}, reply: llm.Reply{
			Thinking:   "先想想怎么查",
			Calls:      []chat.ToolCall{{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"内部消息"}`)}},
			StopReason: "tool_calls",
		}},
		scriptCall{deltas: []string{"查完", "了"}, reply: llm.Reply{StopReason: "stop"}},
	)
	echoTool(registry, t)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	err := agent.SubmitFollowup("查一下")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	events := agent.Book().Events()
	got := kinds(events)

	// 第二步的模型请求里必须带着工具结果（模型看得见）。
	requests := adapter.sawRequests()
	if len(requests) != 2 {
		t.Fatalf("该问两次模型：got %d 次", len(requests))
	}
	second := requests[1].Messages
	toolSeen := false
	assistantSeen := false
	for _, message := range second {
		if message.Role == "assistant" && message.Text == "我查查" && message.Thinking == "先想想怎么查" && len(message.Calls) == 1 && message.Calls[0].ID == "c1" {
			assistantSeen = true
		}
		if message.Result != nil && message.Result.Output == "内部消息" {
			toolSeen = true
		}
	}
	if !toolSeen {
		t.Fatalf("第二步请求该带工具结果：%+v", second)
	}
	if !assistantSeen {
		t.Fatalf("第二步请求该把 thinking 和工具调用放回同一条助手消息：%+v", second)
	}

	wantSequence := []string{
		session.KindToolCall, session.KindToolStart, session.KindToolResult,
	}
	var flat []string
	for _, kind := range got {
		for _, w := range wantSequence {
			_ = w
		}
		flat = append(flat, kind)
	}
	if !strings.Contains(strings.Join(flat, ","), "tool/call,tool/start,tool/result") {
		t.Fatalf("工具三笔账该按顺序在：got %v", flat)
	}
	if strings.Count(strings.Join(flat, ","), "step/start") != 2 {
		t.Fatalf("带工具的轮该有两步：got %v", flat)
	}
}

func TestFollowupQueuesWhileBusy(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"第一轮"}, reply: llm.Reply{StopReason: "stop"}},
		scriptCall{deltas: []string{"第二轮"}, reply: llm.Reply{StopReason: "stop"}},
	)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	// 两份一起投：第一份开轮时第二份只能在队列里等。
	err := agent.SubmitFollowup("第一件事")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	err = agent.SubmitFollowup("第二件事")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	events := agent.Book().Events()
	count := 0
	for _, event := range events {
		if event.Kind == session.KindTurnStart {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("两份 followup 该开两轮：got %d 轮", count)
	}
}

func TestSteerJoinsNextStepWhileBusy(t *testing.T) {
	_, roster, adapter, registry := newTestStack(t,
		scriptCall{deltas: []string{"先干着"}, reply: llm.Reply{
			Calls:      []chat.ToolCall{{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`)}},
			StopReason: "tool_calls",
		}},
		scriptCall{deltas: []string{"收到"}, reply: llm.Reply{StopReason: "stop"}},
	)
	echoTool(registry, t)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	adapter.mu.Lock()
	adapter.block = make(chan struct{})
	adapter.mu.Unlock()

	err := agent.SubmitFollowup("第一件事")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}

	// 等第一轮真的忙起来（第一问被卡住 = 忙），再捎话。
	deadline := time.Now().Add(2 * time.Second)
	for agent.State() != "busy" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if agent.State() != "busy" {
		t.Fatal("没等到忙起来")
	}

	err = agent.Steer("改用中文回答")
	if err != nil {
		t.Fatalf("捎话失败：%v", err)
	}

	adapter.mu.Lock()
	close(adapter.block)
	adapter.mu.Unlock()

	agent.WaitIdle()

	requests := adapter.sawRequests()
	if len(requests) != 2 {
		t.Fatalf("该问两次：%d", len(requests))
	}
	steered := false
	for _, message := range requests[1].Messages {
		if message.Role == "user" && message.Text == "改用中文回答" {
			steered = true
		}
	}
	if !steered {
		t.Fatalf("中途话该进当前轮的下一步：%+v", requests[1].Messages)
	}
}

func TestMemoGoesToContextNotHistory(t *testing.T) {
	_, roster, adapter, _ := newTestStack(t,
		scriptCall{deltas: []string{"知道了"}, reply: llm.Reply{StopReason: "stop"}},
	)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	err := agent.InjectMemo("现在是周三，用户在赶时间")
	if err != nil {
		t.Fatalf("塞小抄失败：%v", err)
	}
	if agent.State() != "idle" {
		t.Fatal("小抄不吵醒，agent 该还是闲的")
	}

	err = agent.SubmitFollowup("帮我安排")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	// 模型请求里看得到小抄（system 消息）。
	requests := adapter.sawRequests()
	memoSeen := false
	for _, message := range requests[0].Messages {
		if message.Role == "system" && strings.Contains(message.Text, "赶时间") {
			memoSeen = true
		}
	}
	if !memoSeen {
		t.Fatalf("小抄该拼进请求：%+v", requests[0].Messages)
	}

	// 但账本的历史里没有小抄——它不是用户的话。
	for _, message := range agent.Book().ModelHistory() {
		if strings.Contains(message.Text, "赶时间") {
			t.Fatalf("小抄不该进模型历史：%+v", message)
		}
	}
	// 小抄的两条账（投递+领出）在。
	joined := strings.Join(kinds(agent.Book().Events()), ",")
	if !strings.Contains(joined, "inbox/deliver") || !strings.Contains(joined, "inbox/claim") {
		t.Fatalf("小抄该有投递和领出两条账：%v", joined)
	}
}

func TestWaitIdleReturnsImmediatelyWhenIdle(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"好"}, reply: llm.Reply{StopReason: "stop"}},
	)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	done := make(chan struct{})
	go func() {
		agent.WaitIdle()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("本来就闲，WaitIdle 该立刻返回")
	}
}

func TestSystemPromptAndModelReachRequest(t *testing.T) {
	_, roster, adapter, _ := newTestStack(t,
		scriptCall{deltas: []string{"嗯"}, reply: llm.Reply{StopReason: "stop"}},
	)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "deepseek-chat", SystemPrompt: "你是管家"})

	err := agent.SubmitFollowup("在吗")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	request := adapter.sawRequests()[0]
	if request.Model != "deepseek-chat" {
		t.Fatalf("型号该透传：got %q", request.Model)
	}
	if len(request.Messages) == 0 || request.Messages[0].Role != "system" || request.Messages[0].Text != "你是管家" {
		t.Fatalf("系统提示该在最前：%+v", request.Messages)
	}

	// 快照存的就是当年真发出去的那份。
	var snapshot session.SnapshotData
	for _, event := range agent.Book().Events() {
		if event.Kind == session.KindSnapshot {
			err = json.Unmarshal(event.Data, &snapshot)
			if err != nil {
				t.Fatalf("解不开快照：%v", err)
			}
		}
	}
	if !strings.Contains(string(snapshot.Request), "你是管家") || !strings.Contains(string(snapshot.Request), "deepseek-chat") {
		t.Fatalf("快照该是完整请求原文：got %s", snapshot.Request)
	}
}

func TestCloseStopsDriver(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	conversation := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})
	err := conversation.Close()
	if err != nil {
		t.Fatalf("关闭会话失败：%v", err)
	}
	err = conversation.Close()
	if err != nil {
		t.Fatalf("重复关闭不该失败：%v", err)
	}

	_, err = roster.GetSession("小红")
	if err == nil {
		t.Fatal("除名后该取不到")
	}
}

func TestRosterRejectsDuplicateID(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	_ = startTestSession(t, roster, "小红", AgentConfig{Model: "m"})
	_, err := roster.StartSession("小红", "小红")
	if err == nil {
		t.Fatal("同名会话该报错")
	}
}

func waitForState(t *testing.T, agent *Conversation, state string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for agent.State() != state && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if agent.State() != state {
		t.Fatalf("没等到 %s，当前 %s", state, agent.State())
	}
}

// event 手工造一笔旧账（seed 用）。
func event(kind string, seq int, data any) session.Event {
	raw, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return session.Event{Kind: kind, Seq: seq, Time: 1, Data: raw}
}

func TestCancelFinalizesPartialReply(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"说了", "半句"}, hang: true},
	)

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	err := agent.SubmitFollowup("讲个故事")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	waitForState(t, agent, "busy")
	agent.Cancel()
	agent.WaitIdle()

	history := agent.Book().ModelHistory()
	if len(history) != 2 {
		t.Fatalf("该是 user+被打断的半句：%+v", history)
	}
	last := history[1]
	if last.Role != "assistant" || last.Text != "说了半句" || !last.Interrupted {
		t.Fatalf("半句该固化成被打断的定稿：%+v", last)
	}

	joined := strings.Join(kinds(agent.Book().Events()), ",")
	if !strings.Contains(joined, "assistant/final") || !strings.Contains(joined, "step/end") || !strings.Contains(joined, "turn/end") {
		t.Fatalf("打断的账要收口干净：%v", joined)
	}
}

func TestCancelWithRunningToolLeavesUnknown(t *testing.T) {
	_, roster, _, registry := newTestStack(t,
		scriptCall{deltas: []string{"我调工具"}, reply: llm.Reply{
			Calls:      []chat.ToolCall{{ID: "c1", Name: "slow", Argument: json.RawMessage(`{}`)}},
			StopReason: "tool_calls",
		}},
	)

	err := registry.Register(tools.Tool{
		Schema: chat.ToolSchema{Name: "slow", Description: "慢工具", Parameters: []byte(`{"type":"object"}`)},
		Execute: func(ctx context.Context, arguments json.RawMessage) (string, error) {
			<-ctx.Done()
			return "干到一半", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"})

	err = agent.SubmitFollowup("跑个慢工具")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	waitForState(t, agent, "busy")
	agent.Cancel()
	agent.WaitIdle()

	got := kinds(agent.Book().Events())
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "tool/call") || !strings.Contains(joined, "tool/start") {
		t.Fatalf("开跑过的调用账上要有：got %v", got)
	}
	if strings.Contains(joined, "tool/result") {
		t.Fatalf("结果不明不许补结果——留待恢复：%v", got)
	}
}

func TestRecoverRestoresPendingDeliveries(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"接着办"}, reply: llm.Reply{StopReason: "stop"}},
	)

	seed := []session.Event{
		event(session.KindDeliver, 1, session.DeliverData{ID: "d1", Text: "旧问题", Target: TargetNextTurn}),
		event(session.KindClaim, 2, session.ClaimData{ID: "d1"}),
		event(session.KindUserMessage, 3, session.UserMessageData{Text: "旧问题"}),
		event(session.KindTurnStart, 4, struct{}{}),
		event(session.KindTurnEnd, 5, struct{}{}),
		event(session.KindDeliver, 6, session.DeliverData{ID: "d2", Text: "重启前没来得及办", Target: TargetNextTurn}),
	}

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"}, seed...)
	agent.WaitIdle()

	joined := strings.Join(kinds(agent.Book().Events()), ",")
	if !strings.Contains(joined, "inbox/claim") || !strings.Contains(joined, session.KindUserMessage) {
		t.Fatalf("找回的待办该接着办：%v", joined)
	}
	found := false
	for _, message := range agent.Book().ModelHistory() {
		if message.Text == "重启前没来得及办" {
			found = true
		}
	}
	if !found {
		t.Fatalf("没领出的消息一条不丢：%+v", agent.Book().ModelHistory())
	}
}

func TestRecoverContinuesDeliveryIDs(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"旧消息办完"}, reply: llm.Reply{StopReason: "stop"}},
		scriptCall{deltas: []string{"新消息办完"}, reply: llm.Reply{StopReason: "stop"}},
	)

	seed := []session.Event{
		event(session.KindDeliver, 1, session.DeliverData{ID: "d1", Text: "重启前没办", Target: TargetNextTurn}),
	}

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"}, seed...)
	agent.WaitIdle()

	err := agent.SubmitFollowup("重启后的新消息")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	var deliveryIDs []string
	for _, event := range agent.Book().Events() {
		if event.Kind != session.KindDeliver {
			continue
		}
		var data session.DeliverData
		err = json.Unmarshal(event.Data, &data)
		if err != nil {
			t.Fatalf("投递解不开：%v", err)
		}
		deliveryIDs = append(deliveryIDs, data.ID)
	}

	want := []string{"d1", "d2"}
	if len(deliveryIDs) != len(want) {
		t.Fatalf("投递编号数量不对：got %v, want %v", deliveryIDs, want)
	}
	for i := range want {
		if deliveryIDs[i] != want[i] {
			t.Fatalf("重启后编号该接着旧账往后数：got %v, want %v", deliveryIDs, want)
		}
	}
}

func TestRecoverFillsDanglingStartedTool(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	seed := []session.Event{
		event(session.KindDeliver, 1, session.DeliverData{ID: "d1", Text: "发封邮件", Target: TargetNextTurn}),
		event(session.KindClaim, 2, session.ClaimData{ID: "d1"}),
		event(session.KindUserMessage, 3, session.UserMessageData{Text: "发封邮件"}),
		event(session.KindTurnStart, 4, struct{}{}),
		event(session.KindStepStart, 5, struct{}{}),
		event(session.KindSnapshot, 6, session.SnapshotData{Request: []byte(`{}`)}),
		event(session.KindChunk, 7, session.ChunkData{Delta: "我发"}),
		event(session.KindAssistantFinal, 8, session.AssistantFinalData{Text: "我发"}),
		event(session.KindToolCall, 9, session.ToolCallData{ID: "c1", Name: "mail"}),
		event(session.KindToolStart, 10, session.ToolStartData{CallID: "c1"}),
		// 崩：c1 开跑了没回话，轮步没收口，还有一句半话没定稿
		event(session.KindChunk, 11, session.ChunkData{Delta: "邮件"}),
	}

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"}, seed...)

	events := agent.Book().Events()
	var lastResult *session.ToolResultData
	for _, e := range events {
		if e.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err := json.Unmarshal(e.Data, &data)
		if err != nil {
			t.Fatalf("解不开：%v", err)
		}
		lastResult = &data
	}
	if lastResult == nil {
		t.Fatal("悬空的调用该补一笔结果")
	}
	if lastResult.CallID != "c1" || lastResult.Status != "unknown" {
		t.Fatalf("开跑过的该补结果不明：got %+v", lastResult)
	}
	if !strings.Contains(lastResult.Output, "结果不明") {
		t.Fatalf("正文该是写给模型的决策指引：got %q", lastResult.Output)
	}

	history := agent.Book().ModelHistory()
	last := history[len(history)-1]
	if !last.Interrupted || last.Text != "邮件" {
		t.Fatalf("悬空的半句该固化成被打断定稿：%+v", last)
	}

	joined := strings.Join(kinds(events), ",")
	if !strings.Contains(joined, "step/end") || !strings.Contains(joined, "turn/end") {
		t.Fatalf("悬空的轮步该补收口：%v", joined)
	}
}

func TestRecoverSkipsUnstartedDanglingTool(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	seed := []session.Event{
		event(session.KindTurnStart, 1, struct{}{}),
		event(session.KindStepStart, 2, struct{}{}),
		event(session.KindToolCall, 3, session.ToolCallData{ID: "c2", Name: "bash"}),
		// 崩：c2 还没开跑
	}

	agent := startTestSession(t, roster, "小红", AgentConfig{Model: "m"}, seed...)

	for _, e := range agent.Book().Events() {
		if e.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err := json.Unmarshal(e.Data, &data)
		if err != nil {
			t.Fatalf("解不开：%v", err)
		}
		if data.CallID != "c2" || data.Status != session.ResultSkipped {
			t.Fatalf("没开跑的该补跳过：got %+v", data)
		}
	}
}
