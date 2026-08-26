package loop

import (
	"testing"
	"time"

	"harness/agents"
	"harness/chat"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

func TestRosterResumesExistingAgent(t *testing.T) {
	journal := session.NewMemoryJournal()
	profiles := newMemoryProfileStore()
	firstApp, firstRoster, _, _ := newTestStackWithStores(t, journal, profiles)
	first := startTestSession(t, firstRoster, "小刚", RunConfig{})
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
		if appended.Event.Kind != session.KindToolResult {
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
	if adapter.sawRequests()[0].Model != "old-model" {
		t.Fatalf("更新模式不能暗中改会话模型：%+v", adapter.sawRequests()[0])
	}
}

func TestConversationRecordsSelectedModelSeparatelyFromPreset(t *testing.T) {
	_, roster, adapter, _ := newTestStack(t, scriptCall{reply: llm.Reply{Text: "好", StopReason: "stop"}})
	err := roster.CreateAgent(AgentProfile{ID: "极简", Provider: "剧本", Model: "旧模型", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := roster.StartSession("极简", "模型独立")
	if err != nil {
		t.Fatal(err)
	}
	err = conversation.SelectModel(llm.Selection{Provider: "剧本", Model: "新模型", Thinking: "off"})
	if err != nil {
		t.Fatal(err)
	}
	err = conversation.SubmitFollowup("你好")
	if err != nil {
		t.Fatal(err)
	}
	conversation.WaitIdle()
	if adapter.sawRequests()[0].Model != "新模型" {
		t.Fatalf("模型切换没有用于下一轮：%+v", adapter.sawRequests()[0])
	}
	selected, found := conversation.Book().LastModelSelection()
	if !found || selected.Model != "新模型" {
		t.Fatalf("账本没有记住最后模型：%+v found=%v", selected, found)
	}
}

func TestOnePresetCanRunTwoIndependentSessions(t *testing.T) {
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
	if first.SessionID() == second.SessionID() {
		t.Fatalf("两段会话必须有不同身份：%s / %s", first.SessionID(), second.SessionID())
	}
	header := first.Book().Header()
	if header.PresetID != "小红" || header.PresetRevision != 1 {
		t.Fatalf("会话封面没锁住模式版本：%+v", header)
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
	if resumed.Book().Header().PresetID != "小红" {
		t.Fatalf("恢复后模式不对：%+v", resumed.Book().Header())
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
	_, visible := registry.Lookup("hidden", first.SessionID())
	if visible {
		t.Fatal("白名单外工具不该给 Agent 看见")
	}
	err = first.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, visible = registry.Lookup("hidden", second.SessionID())
	if visible {
		t.Fatal("关一段会话不该改变另一段会话的白名单")
	}
	err = second.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, visible = registry.Lookup("hidden", second.SessionID())
	if !visible {
		t.Fatal("会话关闭后，它的专属小表应该收掉")
	}
}

func TestResumeUsesLockedPresetRevision(t *testing.T) {
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
	if request.Model != "old-model" || request.Messages[0].Text != "旧人设" {
		t.Fatalf("恢复后没有使用锁定的模式版本：%+v", request)
	}
}

// echoTool 一个回显工具，给工具闭环用。

func TestRosterAllowsDuplicateTitles(t *testing.T) {
	_, roster, _, _ := newTestStack(t)

	first := startTestSession(t, roster, "小红", RunConfig{Model: "m"})
	second, err := roster.StartSession("小红", "小红")
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID() == second.SessionID() {
		t.Fatal("同名会话也必须有不同内部身份")
	}
}
