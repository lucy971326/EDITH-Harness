package loop

import (
	"encoding/json"
	"strings"
	"testing"

	"harness/llm"
	"harness/session"
)

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

	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"}, seed...)
	conversation.WaitIdle()

	joined := strings.Join(kinds(conversation.Book().Events()), ",")
	if !strings.Contains(joined, "inbox/claim") || !strings.Contains(joined, session.KindUserMessage) {
		t.Fatalf("找回的待办该接着办：%v", joined)
	}
	found := false
	for _, message := range conversation.Book().ModelHistory() {
		if message.Text == "重启前没来得及办" {
			found = true
		}
	}
	if !found {
		t.Fatalf("没领出的消息一条不丢：%+v", conversation.Book().ModelHistory())
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

	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"}, seed...)
	conversation.WaitIdle()

	err := conversation.SubmitFollowup("重启后的新消息")
	if err != nil {
		t.Fatalf("投递失败：%v", err)
	}
	conversation.WaitIdle()

	var deliveryIDs []string
	for _, event := range conversation.Book().Events() {
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

	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"}, seed...)

	events := conversation.Book().Events()
	var lastResult *session.ToolResultData
	for _, event := range events {
		if event.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err := json.Unmarshal(event.Data, &data)
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

	history := conversation.Book().ModelHistory()
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

	conversation := startTestSession(t, roster, "小红", RunConfig{Model: "m"}, seed...)

	for _, event := range conversation.Book().Events() {
		if event.Kind != session.KindToolResult {
			continue
		}
		var data session.ToolResultData
		err := json.Unmarshal(event.Data, &data)
		if err != nil {
			t.Fatalf("解不开：%v", err)
		}
		if data.CallID != "c2" || data.Status != session.ResultSkipped {
			t.Fatalf("没开跑的该补跳过：got %+v", data)
		}
	}
}
