package loop

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// silentBroadcaster 给账本一个不出声的广播口。
type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

// memoryJournalPlugin 给 loop 测试装内存账本，避免测试碰本地文件。
type memoryJournalPlugin struct{}

func (memoryJournalPlugin) Name() string { return "test-memory-journal" }

func (memoryJournalPlugin) Start(app *core.App) error {
	app.RegisterService("journal", session.NewMemoryJournal())
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
	t.Helper()

	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(
		memoryJournalPlugin{},
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
	err = llmSvc.SetDefault("剧本")
	if err != nil {
		t.Fatalf("设默认失败：%v", err)
	}

	registry, err := tools.Get(app)
	if err != nil {
		t.Fatalf("取工具登记处失败：%v", err)
	}

	err = app.Install(Plugin{})
	if err != nil {
		t.Fatalf("装 loop 插件失败：%v", err)
	}
	roster, err := Get(app)
	if err != nil {
		t.Fatalf("取 agent 名册失败：%v", err)
	}

	return app, roster, adapter, registry
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

	agent, err := roster.Create("小红", AgentConfig{Model: "test-model", SystemPrompt: "你是测试员"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	err = agent.SubmitFollowup("你好")
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
			Calls:      []chat.ToolCall{{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"内部消息"}`)}},
			StopReason: "tool_calls",
		}},
		scriptCall{deltas: []string{"查完", "了"}, reply: llm.Reply{StopReason: "stop"}},
	)
	echoTool(registry, t)

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	err = agent.SubmitFollowup("查一下")
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
	for _, message := range second {
		if message.Result != nil && message.Result.Output == "内部消息" {
			toolSeen = true
		}
	}
	if !toolSeen {
		t.Fatalf("第二步请求该带工具结果：%+v", second)
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	// 两份一起投：第一份开轮时第二份只能在队列里等。
	err = agent.SubmitFollowup("第一件事")
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	adapter.mu.Lock()
	adapter.block = make(chan struct{})
	adapter.mu.Unlock()

	err = agent.SubmitFollowup("第一件事")
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	err = agent.InjectMemo("现在是周三，用户在赶时间")
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

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

	agent, err := roster.Create("小红", AgentConfig{Model: "deepseek-chat", SystemPrompt: "你是管家"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	err = agent.SubmitFollowup("在吗")
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

	_, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	roster.Drop("小红")

	_, err = roster.Get("小红")
	if err == nil {
		t.Fatal("除名后该取不到")
	}
}

func TestRosterRejectsDuplicateID(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	_, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}
	_, err = roster.Create("小红", AgentConfig{Model: "m"})
	if err == nil {
		t.Fatal("同名该报错")
	}
}

func waitForState(t *testing.T, agent *Agent, state string) {
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

	err = agent.SubmitFollowup("讲个故事")
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"})
	if err != nil {
		t.Fatalf("开 agent 失败：%v", err)
	}

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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"}, seed...)
	if err != nil {
		t.Fatalf("恢复失败：%v", err)
	}
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"}, seed...)
	if err != nil {
		t.Fatalf("恢复失败：%v", err)
	}
	agent.WaitIdle()

	err = agent.SubmitFollowup("重启后的新消息")
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"}, seed...)
	if err != nil {
		t.Fatalf("恢复失败：%v", err)
	}

	events := agent.Book().Events()
	var lastResult *session.ToolResultData
	for _, e := range events {
		if e.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err = json.Unmarshal(e.Data, &data)
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

	agent, err := roster.Create("小红", AgentConfig{Model: "m"}, seed...)
	if err != nil {
		t.Fatalf("恢复失败：%v", err)
	}

	for _, e := range agent.Book().Events() {
		if e.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err = json.Unmarshal(e.Data, &data)
		if err != nil {
			t.Fatalf("解不开：%v", err)
		}
		if data.CallID != "c2" || data.Status != session.ResultSkipped {
			t.Fatalf("没开跑的该补跳过：got %+v", data)
		}
	}
}
