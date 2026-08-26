package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

func TestHappyTurnFullLedger(t *testing.T) {
	_, roster, _, registry := newTestStack(t,
		scriptCall{deltas: []string{"好", "的"}, reply: llm.Reply{StopReason: "stop"}},
	)
	echoTool(registry, t)

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "test-model", SystemPrompt: "你是测试员"})

	err := agent.SubmitFollowup("你好")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	agent.WaitIdle()

	events := agent.Book().Events()
	got := kinds(events)
	want := []string{
		session.KindModelSelected, session.KindDeliver, session.KindClaim, session.KindUserMessage,
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

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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

func TestWaitIdleReturnsImmediatelyWhenIdle(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"好"}, reply: llm.Reply{StopReason: "stop"}},
	)

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "deepseek-chat", SystemPrompt: "你是管家"})

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

	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"})
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
	select {
	case <-conversation.driver.done:
	case <-time.After(time.Second):
		t.Fatal("关闭会话后搬运工必须退出")
	}
}

func TestCloseCancelsBusyDriverBeforeReleasingSession(t *testing.T) {
	_, roster, _, _ := newTestStack(t, scriptCall{hang: true})
	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"})
	err := conversation.SubmitFollowup("别停")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, conversation, "busy")
	done := make(chan error, 1)
	go func() {
		done <- conversation.Close()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("关闭忙会话必须先取消并等搬运工退出")
	}
}

func TestLoopBroadcastsConversationError(t *testing.T) {
	app, roster, _, _ := newTestStack(t)
	seen := make(chan agents.ConversationError, 1)
	app.Subscribe(agents.EventConversationError, func(payload any) {
		seen <- payload.(agents.ConversationError)
	})
	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"})
	err := conversation.SubmitFollowup("会失败")
	if err != nil {
		t.Fatal(err)
	}
	conversation.WaitIdle()
	select {
	case problem := <-seen:
		if problem.SessionID != conversation.SessionID() || problem.Message == "" {
			t.Fatalf("实时错误通知不完整：%+v", problem)
		}
	case <-time.After(time.Second):
		t.Fatal("模型失败时终端应收到实时错误通知")
	}
}

func TestCancelFinalizesPartialReply(t *testing.T) {
	_, roster, _, _ := newTestStack(t,
		scriptCall{deltas: []string{"说了", "半句"}, hang: true},
	)

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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

func TestCancelBeforeStepContextCancelsNextStep(t *testing.T) {
	agent := &Conversation{working: true}
	if agent.State() != "starting" {
		t.Fatalf("接活但尚未开步时应报告 starting：got %s", agent.State())
	}
	agent.Cancel()
	ctx := agent.stepContext()
	defer agent.clearStepContext()
	if ctx.Err() != context.Canceled {
		t.Fatalf("步骤上下文应继承提前到达的取消请求：err=%v", ctx.Err())
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
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			<-ctx.Done()
			return "干到一半", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	agent := startTestSession(t, roster, "小红", RunConfig{Model: "m"})

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
