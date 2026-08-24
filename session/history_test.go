package session

import (
	"encoding/json"
	"reflect"
	"testing"

	"harness/chat"
)

// 记一段剧本：用户问好、模型流式回复带一次工具调用、回话、用户再说话。
func recordScript(t *testing.T, s *Session) {
	t.Helper()

	steps := []func() (Event, error){
		s.RecordTurnStart,
		func() (Event, error) { return s.RecordUserMessage("你好") },
		s.RecordStepStart,
		func() (Event, error) { return s.RecordChunk("你") },
		func() (Event, error) { return s.RecordChunk("好！") },
		func() (Event, error) {
			return s.RecordToolCall(ToolCallData{ID: "c1", Name: "bash", Argument: json.RawMessage(`{"cmd":"ls"}`)})
		},
		s.RecordStepEnd,
		func() (Event, error) { return s.RecordToolStart("c1") },
		func() (Event, error) {
			return s.RecordToolResult(ToolResultData{CallID: "c1", Output: "file.txt", Status: ResultSuccess})
		},
		func() (Event, error) { return s.RecordUserMessage("谢了") },
		s.RecordTurnEnd,
	}
	for i, step := range steps {
		_, err := step()
		if err != nil {
			t.Fatalf("剧本第 %d 步记账失败：%v", i+1, err)
		}
	}
}

func TestModelHistoryFullScript(t *testing.T) {
	_, s, _ := newTestStore(t)
	recordScript(t, s)

	history := s.ModelHistory()
	if len(history) != 4 {
		t.Fatalf("该是 4 条消息（user/assistant/tool/user）：got %d", len(history))
	}

	if history[0].Role != "user" || history[0].Text != "你好" {
		t.Fatalf("第 1 条该是用户问好：got %+v", history[0])
	}
	assistant := history[1]
	if assistant.Role != "assistant" || assistant.Text != "你好！" {
		t.Fatalf("第 2 条该是折叠后的助手回复：got %+v", assistant)
	}
	if len(assistant.Calls) != 1 || assistant.Calls[0].ID != "c1" || assistant.Calls[0].Name != "bash" {
		t.Fatalf("助手消息该带上工具调用：got %+v", assistant.Calls)
	}
	if string(assistant.Calls[0].Argument) != `{"cmd":"ls"}` {
		t.Fatalf("参数该原样保留：got %s", assistant.Calls[0].Argument)
	}
	if history[2].Role != "tool" || history[2].Result.CallID != "c1" || history[2].Result.Output != "file.txt" {
		t.Fatalf("第 3 条该是工具回话：got %+v", history[2])
	}
	if history[3].Role != "user" || history[3].Text != "谢了" {
		t.Fatalf("第 4 条该是用户道谢：got %+v", history[3])
	}
}

func TestModelHistoryFoldsOnlyConsecutiveChunks(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordChunk("第一段")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordStepEnd()
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordChunk("第二段")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	history := s.ModelHistory()
	if len(history) != 2 {
		t.Fatalf("步边界两侧的字不该拼成一句：got %d 条", len(history))
	}
	if history[0].Text != "第一段" || history[1].Text != "第二段" {
		t.Fatalf("两条各自折叠：got %+v", history)
	}
}

func TestModelHistoryIgnoresInvisibleKinds(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordTurnStart()
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordDeliver("d1", "排队中的话", "next-turn")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordClaim("d1")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordToolStart("c1")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordSnapshot([]byte(`[{"role":"user"}]`))
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordTurnEnd()
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	if len(s.ModelHistory()) != 0 {
		t.Fatalf("轮步/投递/领出/快照不该进模型历史：got %v", s.ModelHistory())
	}
	if len(s.Events()) != 6 {
		t.Fatalf("但账上要一笔不少：got %d 笔", len(s.Events()))
	}
}

func TestSummaryReplacesOldEvents(t *testing.T) {
	_, s, _ := newTestStore(t)

	_, err := s.RecordUserMessage("你好")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordChunk("你也好")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordStepEnd()
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	_, err = s.RecordUserMessage("再见")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	_, err = s.RecordSummary(SummaryData{Text: "用户问好、寒暄、道别"}, []int{1, 2, 4})
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}

	history := s.ModelHistory()
	if len(history) != 1 {
		t.Fatalf("被收编的该只剩摘要：got %v", history)
	}
	if history[0].Role != "user" || history[0].Text != "用户问好、寒暄、道别" {
		t.Fatalf("摘要该变成模型可见的一条：got %+v", history[0])
	}
	if len(s.Events()) != 5 {
		t.Fatalf("账本只增不改，旧账一笔不丢：got %d 笔", len(s.Events()))
	}
}

func TestReplayRebuildsIdenticalHistory(t *testing.T) {
	store, s, _ := newTestStore(t)
	recordScript(t, s)

	replayed, err := store.Create("重放的账", "测试 Agent", 1, s.Events()...)
	if err != nil {
		t.Fatalf("重放失败：%v", err)
	}

	if !reflect.DeepEqual(s.ModelHistory(), replayed.ModelHistory()) {
		t.Fatalf("重放出的历史该一模一样：\n原：%+v\n新：%+v", s.ModelHistory(), replayed.ModelHistory())
	}
	if !reflect.DeepEqual(s.Events(), replayed.Events()) {
		t.Fatal("重放出的账本该一模一样")
	}
}

func TestOpenRestoresExistingBook(t *testing.T) {
	journal := NewMemoryJournal()
	firstStore := NewStore(journal, &recordingBroadcaster{})
	first, err := firstStore.Create("旧账", "测试 Agent", 1)
	if err != nil {
		t.Fatal(err)
	}
	recordScript(t, first)

	secondStore := NewStore(journal, &recordingBroadcaster{})
	opened, err := secondStore.Open("旧账")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Events(), opened.Events()) {
		t.Fatal("打开旧账后，所有事件都该回来")
	}
	if !reflect.DeepEqual(first.ModelHistory(), opened.ModelHistory()) {
		t.Fatal("打开旧账后，模型历史也该一模一样")
	}

	_, err = secondStore.Create("旧账", "测试 Agent", 1)
	if err == nil {
		t.Fatal("已经打开的账本不能再当新账创建")
	}
}

func TestOpenRejectsLegacyHeader(t *testing.T) {
	journal := NewMemoryJournal()
	err := journal.Create("旧账", Header{FormatVersion: 1, ID: "旧账"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(journal, &recordingBroadcaster{})
	_, err = store.Open("旧账")
	if err == nil {
		t.Fatal("没有 Agent 归属的旧账应该拒绝打开")
	}
}

func TestReplayRejectsBrokenSeq(t *testing.T) {
	store, _, _ := newTestStore(t)

	seed := []Event{
		{Kind: KindUserMessage, Seq: 1, Data: []byte(`{"Text":"你好"}`)},
		{Kind: KindUserMessage, Seq: 3, Data: []byte(`{"Text":"编号跳了"}`)},
	}
	_, err := store.Create("断号的账", "测试 Agent", 1, seed...)
	if err == nil {
		t.Fatal("编号断了的重放该被拒绝")
	}
}

func TestReplayRejectsUnknownRequiredKind(t *testing.T) {
	store, _, _ := newTestStore(t)

	seed := []Event{
		{Kind: KindUserMessage, Seq: 1, Data: []byte(`{"Text":"你好"}`)},
		{Kind: "ghost/critical", Seq: 2, Data: []byte(`{"x":1}`), SkipIfUnknown: false},
	}
	_, err := store.Create("带幽灵的账", "测试 Agent", 1, seed...)
	if err == nil {
		t.Fatal("不认识且必需的事件该让整本账拒读")
	}
}

func TestReplaySkipsUnknownOptionalKind(t *testing.T) {
	store, _, _ := newTestStore(t)

	seed := []Event{
		{Kind: KindUserMessage, Seq: 1, Data: []byte(`{"Text":"你好"}`)},
		{Kind: "ghost/note", Seq: 2, Data: []byte(`{"x":1}`), SkipIfUnknown: true},
		{Kind: KindUserMessage, Seq: 3, Data: []byte(`{"Text":"还在"}`)},
	}
	s, err := store.Create("带过客的账", "测试 Agent", 1, seed...)
	if err != nil {
		t.Fatalf("可跳过的事件不该挡住重放：%v", err)
	}

	history := s.ModelHistory()
	if len(history) != 2 {
		t.Fatalf("幽灵事件被跳过，其余照常：got %v", history)
	}
	if len(s.Events()) != 3 {
		t.Fatalf("跳过不等于删除，账上还得有：got %d 笔", len(s.Events()))
	}
}

func TestCacheRecomputesOnlyOnReplace(t *testing.T) {
	_, s, _ := newTestStore(t)

	for i := 0; i < 5; i++ {
		_, err := s.RecordChunk("第几段")
		if err != nil {
			t.Fatalf("记账失败：%v", err)
		}
	}
	if s.history.rebuilds != 0 {
		t.Fatalf("普通记账该走增量，不该重算：got %d 次", s.history.rebuilds)
	}

	_, err := s.RecordSummary(SummaryData{Text: "收编上面"}, []int{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	if s.history.rebuilds != 1 {
		t.Fatalf("取代发生才重算：got %d 次", s.history.rebuilds)
	}

	_, err = s.RecordUserMessage("继续")
	if err != nil {
		t.Fatalf("记账失败：%v", err)
	}
	if s.history.rebuilds != 1 {
		t.Fatalf("没有新的取代不该再重算：got %d 次", s.history.rebuilds)
	}
}

func TestAssistantFinalTakesOverChunks(t *testing.T) {
	_, s, _ := newTestStore(t)

	chunks := []string{"你", "好", "呀"}
	seqs := make([]int, 0, len(chunks))
	for _, delta := range chunks {
		event, err := s.RecordChunk(delta)
		if err != nil {
			t.Fatalf("记账失败：%v", err)
		}
		seqs = append(seqs, event.Seq)
	}

	// 定稿：正常说完，用量入账，素材被收编。
	_, err := s.RecordAssistantFinal(AssistantFinalData{
		Text:  "你好呀",
		Usage: chat.Usage{InputTokens: 10, OutputTokens: 3},
	}, seqs)
	if err != nil {
		t.Fatalf("定稿失败：%v", err)
	}

	history := s.ModelHistory()
	if len(history) != 1 {
		t.Fatalf("定稿后该只有一条消息：got %v", history)
	}
	if history[0].Role != "assistant" || history[0].Text != "你好呀" {
		t.Fatalf("该是定稿文本：got %+v", history[0])
	}
	if history[0].Interrupted {
		t.Fatal("正常说完不算被打断")
	}
	if len(s.Events()) != 4 {
		t.Fatalf("素材在账上要一笔不少：got %d 笔", len(s.Events()))
	}
}

func TestAssistantFinalInterruptedKeepsPrefix(t *testing.T) {
	_, s, _ := newTestStore(t)

	seqs := []int{}
	for _, delta := range []string{"让我查", "一下"} {
		event, err := s.RecordChunk(delta)
		if err != nil {
			t.Fatalf("记账失败：%v", err)
		}
		seqs = append(seqs, event.Seq)
	}

	// 用户打断了模型：已收到的半句固化成一条带标记的消息。
	_, err := s.RecordAssistantFinal(AssistantFinalData{
		Text:        "让我查一下",
		Interrupted: true,
	}, seqs)
	if err != nil {
		t.Fatalf("定稿失败：%v", err)
	}

	history := s.ModelHistory()
	if len(history) != 1 {
		t.Fatalf("被打断的回复只落一条：got %v", history)
	}
	if !history[0].Interrupted || history[0].Text != "让我查一下" {
		t.Fatalf("该是带打断标记的半句：got %+v", history[0])
	}
}

func TestHistoryMergesFinalThinkingAndToolCall(t *testing.T) {
	store, book, _ := newTestStore(t)
	_, err := book.RecordAssistantFinal(AssistantFinalData{Text: "我来写", Thinking: "先确认路径"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordToolCall(ToolCallData{ID: "call-1", Name: "write_file", Argument: json.RawMessage(`{"path":"a.txt"}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = book.RecordToolResult(ToolResultData{CallID: "call-1", Output: "写好了", Status: ResultSuccess})
	if err != nil {
		t.Fatal(err)
	}
	history := book.ModelHistory()
	if len(history) != 2 {
		t.Fatalf("定稿、工具调用和结果应是两条消息：%+v", history)
	}
	if history[0].Role != "assistant" || history[0].Text != "我来写" || history[0].Thinking != "先确认路径" || len(history[0].Calls) != 1 {
		t.Fatalf("助手定稿与工具调用没有合并：%+v", history[0])
	}
	if history[1].Role != "tool" || history[1].Result.CallID != "call-1" {
		t.Fatalf("工具结果没有排在合成助手消息后：%+v", history[1])
	}
	opened, err := store.Create("重放思考", "测试 Agent", 1, book.Events()...)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(history, opened.ModelHistory()) {
		t.Fatalf("重放后 Thinking 和工具调用应保留：%+v", opened.ModelHistory())
	}
}

func TestHistoryKeepsSiblingToolCallsInOneAssistantMessage(t *testing.T) {
	_, book, _ := newTestStore(t)
	_, err := book.RecordAssistantFinal(AssistantFinalData{Text: "我一起做"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []ToolCallData{
		{ID: "call-1", Name: "first", Argument: json.RawMessage(`{}`)},
		{ID: "call-2", Name: "second", Argument: json.RawMessage(`{}`)},
	} {
		_, err = book.RecordToolCall(call)
		if err != nil {
			t.Fatal(err)
		}
		_, err = book.RecordToolResult(ToolResultData{CallID: call.ID, Output: "好了", Status: ResultSuccess})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = book.RecordStepEnd()
	if err != nil {
		t.Fatal(err)
	}

	history := book.ModelHistory()
	if len(history) != 3 {
		t.Fatalf("一条助手消息和两条工具结果：%+v", history)
	}
	if len(history[0].Calls) != 2 || history[0].Calls[0].ID != "call-1" || history[0].Calls[1].ID != "call-2" {
		t.Fatalf("同一次模型回复的工具调用不能拆开：%+v", history[0])
	}
}

func TestOldSessionWithoutThinkingStillOpens(t *testing.T) {
	store, _, _ := newTestStore(t)
	seed := []Event{
		{Kind: KindUserMessage, Seq: 1, Data: json.RawMessage(`{"Text":"旧问题"}`)},
		{Kind: KindAssistantFinal, Seq: 2, Data: json.RawMessage(`{"Text":"旧回复"}`)},
	}
	book, err := store.Create("旧 Thinking", "测试 Agent", 1, seed...)
	if err != nil {
		t.Fatalf("旧账缺 Thinking 仍应兼容：%v", err)
	}
	history := book.ModelHistory()
	if len(history) != 2 || history[1].Text != "旧回复" || history[1].Thinking != "" {
		t.Fatalf("旧账投影不对：%+v", history)
	}
}
