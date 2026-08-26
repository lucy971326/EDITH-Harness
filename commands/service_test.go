package commands

import (
	"context"
	"strings"
	"testing"

	"harness/core"
)

func pingCommand(text string) Command {
	return Command{
		Name:        "ping",
		Description: "探测命令平面",
		Group:       "test",
		Hint:        "可选说明",
		Run: func(ctx context.Context, invocation Invocation) Result {
			return Result{Kind: KindSuccess, Text: text + invocation.RawInput}
		},
	}
}

func TestLooksLikeAndParse(t *testing.T) {
	if !LooksLike("  /ping 你好") {
		t.Fatal("前导空白的斜杠行应进入命令平面")
	}
	if LooksLike("请看 /ping") {
		t.Fatal("中间的斜杠不是命令")
	}
	parsed, ok := Parse("  /ping  你好")
	if !ok || parsed.Name != "ping" || parsed.RawInput != "  你好" {
		t.Fatalf("应保留名字后的原文：%+v ok=%v", parsed, ok)
	}
	_, ok = Parse("/Ping")
	if ok {
		t.Fatal("大写名字不是合法命令")
	}
	_, ok = Parse("/帮助")
	if ok {
		t.Fatal("非 ASCII 名字不是合法命令")
	}
	if !LooksLike("/帮助") {
		t.Fatal("非 ASCII 斜杠行仍在命令平面")
	}
}

func TestRegisterListFindAndUnregister(t *testing.T) {
	registry := NewRegistry()
	unregister, err := registry.Register(pingCommand("pong"))
	if err != nil {
		t.Fatal(err)
	}
	listed := registry.List()
	if len(listed) != 1 || listed[0].Name != "ping" || listed[0].Hint != "可选说明" {
		t.Fatalf("名单不对：%+v", listed)
	}
	command, found := registry.Find("ping")
	if !found || command.Name != "ping" {
		t.Fatal("应能按名找到命令")
	}
	_, err = registry.Register(pingCommand("again"))
	if err == nil || !strings.Contains(err.Error(), "已登记") {
		t.Fatalf("同名应拒绝：%v", err)
	}
	unregister()
	unregister()
	if _, found := registry.Find("ping"); found {
		t.Fatal("撤销后不应还在")
	}
	_, err = registry.Register(pingCommand("pong"))
	if err != nil {
		t.Fatalf("撤销后应能重新登记：%v", err)
	}
}

func TestRegisterRejectsBadMetadata(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(Command{Name: "Ping", Description: "x", Run: func(context.Context, Invocation) Result { return Result{} }})
	if err == nil {
		t.Fatal("大写名字应拒绝")
	}
	_, err = registry.Register(Command{Name: "ping", Run: func(context.Context, Invocation) Result { return Result{} }})
	if err == nil {
		t.Fatal("缺描述应拒绝")
	}
	_, err = registry.Register(Command{Name: "ping", Description: "探测"})
	if err == nil {
		t.Fatal("缺执行本体应拒绝")
	}
}

func TestExecuteKnownUnknownAndInvalid(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(pingCommand("pong"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Execute(context.Background(), "session-1", "/ping 世界")
	if err != nil || result.Kind != KindSuccess || result.Text != "pong 世界" {
		t.Fatalf("已知命令应执行：%+v err=%v", result, err)
	}
	result, err = registry.Execute(context.Background(), "", "/nope")
	if err != nil || result.Kind != KindError || !strings.Contains(result.Text, "/nope") {
		t.Fatalf("未知命令应报错：%+v err=%v", result, err)
	}
	result, err = registry.Execute(context.Background(), "", "/帮助")
	if err != nil || result.Kind != KindError || result.Text != "命令格式不对" {
		t.Fatalf("非法格式应报错：%+v err=%v", result, err)
	}
	_, err = registry.Execute(context.Background(), "", "普通聊天")
	if err == nil {
		t.Fatal("非斜杠行不应走 Execute")
	}
}

func TestExecuteDraftHasEmptySession(t *testing.T) {
	registry := NewRegistry()
	var got Invocation
	_, err := registry.Register(Command{
		Name:        "where",
		Description: "看会话号",
		Run: func(ctx context.Context, invocation Invocation) Result {
			got = invocation
			return Result{Kind: KindSuccess}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Execute(context.Background(), "", "/where")
	if err != nil || result.Kind != KindSuccess || got.SessionID != "" {
		t.Fatalf("草稿应能跑且会话号为空：result=%+v got=%+v err=%v", result, got, err)
	}
}

func TestExecuteCancelledContext(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(pingCommand("pong"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = registry.Execute(ctx, "", "/ping")
	if err == nil {
		t.Fatal("已取消的上下文不应执行")
	}
}

func TestPluginProvidesService(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := Get(app)
	if err != nil {
		t.Fatal(err)
	}
	unregister, err := service.Register(pingCommand("pong"))
	if err != nil {
		t.Fatal(err)
	}
	if len(service.List()) != 1 {
		t.Fatal("插件装上后应能登记")
	}
	unregister()
}
