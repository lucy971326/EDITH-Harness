package terminal

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/loop"
	"harness/persistence/jsonl"
	"harness/persistence/profilejson"
	"harness/session"
	"harness/tools"
	"harness/ui"
)

type echoAdapter struct{}

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

func TestTerminalCreatesChatsAndPrintsFinalOnlyOnce(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	input := bytes.NewBufferString("n\n小红\n1\necho-model\n1\n\n\nn\n第一段\n你好\n:exit\n")
	output := &bytes.Buffer{}
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
	err = app.Install(loop.Plugin{}, Plugin{Input: input, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	screen, err := ui.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = screen.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "小红：收到") != 1 {
		t.Fatalf("流式回复定稿不该重复显示：\n%s", output.String())
	}
}

func TestHistoryHidesReplacedChunksAndThinking(t *testing.T) {
	output := &bytes.Buffer{}
	renderer := newRenderer(output)
	events := []session.Event{
		{Kind: session.KindChunk, Seq: 1, Data: []byte(`{"Delta":"回"}`)},
		{Kind: session.KindChunk, Seq: 2, Data: []byte(`{"Delta":"答"}`)},
		{Kind: session.KindAssistantFinal, Seq: 3, Data: []byte(`{"Text":"回答","Thinking":"不能显示"}`), Replaces: []int{1, 2}},
	}
	renderer.history(events)
	text := output.String()
	if strings.Count(text, "小红：回答") != 1 || strings.Contains(text, "不能显示") {
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

func TestLiveUserMessageDoesNotRepeatTerminalEcho(t *testing.T) {
	output := &bytes.Buffer{}
	renderer := newRenderer(output)
	renderer.prompt("你：")
	renderer.live(session.Event{Kind: session.KindUserMessage, Data: []byte(`{"Text":"你好"}`)})
	if got := output.String(); got != "你：" {
		t.Fatalf("实时用户消息不该重复终端回显：%q", got)
	}
}
