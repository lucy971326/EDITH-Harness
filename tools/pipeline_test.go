package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"harness/chat"
	"harness/core"
	"harness/session"
)

// silentBroadcaster 给账本一个不出声的广播口。
type silentBroadcaster struct{}

func (silentBroadcaster) Broadcast(name string, payload any) {}

// yesApprover / noApprover 是测试用的问人答案。
type yesApprover struct{}

func (yesApprover) Approve(call Call) Decision { return Decision{Kind: Allow} }

type noApprover struct{}

func (noApprover) Approve(call Call) Decision {
	return Decision{Kind: Deny, Reason: "人说不"}
}

// echo 工具：回显参数里的 text 字段。
func echoTool() Tool {
	return Tool{
		Schema: chatSchema("echo", "回显一句话"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			var args struct {
				Text string
			}
			err := json.Unmarshal(arguments, &args)
			if err != nil {
				return "", err
			}
			return args.Text, nil
		},
	}
}

func chatSchema(name string, description string) chat.ToolSchema {
	return chat.ToolSchema{Name: name, Description: description, Parameters: []byte(`{"type":"object"}`)}
}

func newTestApp(t *testing.T) (*core.App, *Registry, *session.Session) {
	t.Helper()

	app := core.New()
	t.Cleanup(app.Close)

	registry := NewRegistry()
	app.RegisterService("tools", registry)

	store := session.NewStore(session.NewMemoryJournal(), silentBroadcaster{})
	book, err := store.Create(session.Header{
		ID:             "测试账",
		Title:          "测试账",
		CreatedAt:      time.Unix(1, 0),
		ProjectID:      "测试项目",
		ProjectRoot:    "/tmp/测试项目",
		PresetID:       "测试模式",
		PresetRevision: 1,
	})
	if err != nil {
		t.Fatalf("开账失败：%v", err)
	}
	return app, registry, book
}

func kinds(events []session.Event) []string {
	var out []string
	for _, event := range events {
		out = append(out, event.Kind)
	}
	return out
}

func TestHappyPathRecordsCallStartResultInOrder(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"你好"}`)})

	if result.Status != session.ResultSuccess || result.Output != "你好" {
		t.Fatalf("该成功回显：got %+v", result)
	}
	want := []string{session.KindToolCall, session.KindToolStart, session.KindToolResult}
	got := kinds(book.Events())
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("账上该是 要调了→开跑→结果：got %v", got)
	}
}

func TestUnknownToolFailsWithoutStart(t *testing.T) {
	app, registry, book := newTestApp(t)

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "没有这工具", Argument: json.RawMessage(`{}`)})

	if result.Status != session.ResultFailed {
		t.Fatalf("该失败：got %+v", result)
	}
	got := kinds(book.Events())
	if len(got) != 2 || got[0] != session.KindToolCall || got[1] != session.KindToolResult {
		t.Fatalf("未知工具也该①⑨闭环，但无 start：got %v", got)
	}
}

func TestPreExecuteDenySkips(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	core.Intercept[PreCall, Decision](app, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		return Decision{Kind: Deny, Reason: "太危险"}
	})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultSkipped || result.Output != "太危险" {
		t.Fatalf("该被前置拒：got %+v", result)
	}
	got := kinds(book.Events())
	if len(got) != 2 || got[1] != session.KindToolResult {
		t.Fatalf("被拒的调用无 start，账上①⑨：got %v", got)
	}
}

func TestGuardDenyBeatsPreExecuteAllow(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	// 前置放行 + 守卫拒绝：守卫是最后一道闸，放行翻不回。
	core.Intercept[PreCall, Decision](app, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		return next(p)
	})
	core.Intercept[Call, string](app, Guard, func(c Call, next func(Call) string) string {
		return "守卫说不"
	})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultSkipped || result.Output != "守卫说不" {
		t.Fatalf("守卫该一票否决：got %+v", result)
	}
	if kinds(book.Events())[1] != session.KindToolResult {
		t.Fatal("被守卫拒的调用无 start")
	}
}

func TestAskWithoutApproverIsDenied(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	core.Intercept[PreCall, Decision](app, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		return Decision{Kind: Ask, Reason: "删文件要问人"}
	})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultSkipped {
		t.Fatalf("问了没人答 = 拒：got %+v", result)
	}
}

func TestAskWithApproverAnswerExecutes(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	core.Intercept[PreCall, Decision](app, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		return Decision{Kind: Ask, Reason: "要确认"}
	})
	app.RegisterService("approval", yesApprover{})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"过审了"}`)})
	if result.Status != session.ResultSuccess || result.Output != "过审了" {
		t.Fatalf("人答了放行该执行：got %+v", result)
	}

	// 换个人答"不"：子作用域用自己的审批人遮蔽全局的。
	agent := app.ForChild("小红")
	agent.RegisterService("approval", noApprover{})
	result = registry.ExecuteCall(context.Background(), agent, book,
		Call{ID: "c2", Name: "echo", Argument: json.RawMessage(`{}`), ScopeID: "小红"})
	if result.Status != session.ResultSkipped || result.Output != "人说不" {
		t.Fatalf("人拒了该拒：got %+v", result)
	}
}

func TestToolPanicBecomesFailedResult(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(Tool{
		Schema: chatSchema("bomb", "会炸的工具"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			panic("炸了")
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	// 走到断言就证明进程没崩。
	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "bomb", Argument: json.RawMessage(`{}`)})

	if result.Status != session.ResultFailed {
		t.Fatalf("panic 该变成 failed：got %+v", result)
	}
	events := book.Events()
	if events[len(events)-1].Kind != session.KindToolResult {
		t.Fatal("崩溃的终局也要入账")
	}
}

func TestExecuteErrorBecomesFailedResult(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(Tool{
		Schema: chatSchema("broken", "必失败的工具"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			return "", errors.New("磁盘满")
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "broken", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultFailed || result.Output != "磁盘满" {
		t.Fatalf("err 该变成 failed：got %+v", result)
	}
}

func TestPostExecuteCanRewriteResult(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	core.Intercept[PostCall, Result](app, PostExecute, func(p PostCall, next func(PostCall) Result) Result {
		runes := []rune(p.Result.Output)
		if len(runes) > 100 {
			p.Result.Output = string(runes[:100]) + "…（太长，已截断）"
		}
		return next(p)
	})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"` + longText(150) + `"}`)})
	if !strings.HasSuffix(result.Output, "…（太长，已截断）") {
		t.Fatalf("后置链该能改写结果：got %q", result.Output[:20])
	}

	final := book.Events()[len(book.Events())-1]
	var data session.ToolResultData
	err = json.Unmarshal(final.Data, &data)
	if err != nil {
		t.Fatalf("解不开结果：%v", err)
	}
	if data.Output != result.Output {
		t.Fatal("账上的终局该是改写后的")
	}
}

func TestAroundChainCanRetry(t *testing.T) {
	app, registry, book := newTestApp(t)
	attempts := 0
	err := registry.Register(Tool{
		Schema: chatSchema("flaky", "第一次必失败的工具"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			attempts++
			if attempts == 1 {
				return "", errors.New("抖了一下")
			}
			return "第二次成了", nil
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	// 重试插件：看到失败就再跑一次本体。
	core.Intercept[Call, Outcome](app, Execute, func(c Call, next func(Call) Outcome) Outcome {
		first := next(c)
		if first.Err != nil {
			return next(c)
		}
		return first
	})

	result := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "flaky", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultSuccess || result.Output != "第二次成了" {
		t.Fatalf("重试该拿到第二次结果：got %+v", result)
	}
	if attempts != 2 {
		t.Fatalf("本体该被跑了两次：got %d", attempts)
	}
}

func TestCancelBeforeStartSkips(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := registry.ExecuteCall(ctx, app, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`)})
	if result.Status != session.ResultSkipped {
		t.Fatalf("开跑前取消该 skipped：got %+v", result)
	}
	got := kinds(book.Events())
	if got[1] != session.KindToolResult {
		t.Fatal("没开跑的取消不能有 start")
	}
}

func TestCancelAfterStartLeavesUnknown(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(Tool{
		Schema: chatSchema("slow", "慢工具"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			select {
			case <-ctx.Done():
				return "干了一半断了", ctx.Err()
			case <-time.After(10 * time.Second):
				return "干完了", nil
			}
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	result := registry.ExecuteCall(ctx, app, book,
		Call{ID: "c1", Name: "slow", Argument: json.RawMessage(`{}`)})

	if result.Status != ResultUnknown {
		t.Fatalf("已开跑被取消该返回 unknown：got %+v", result)
	}
	got := kinds(book.Events())
	if len(got) != 2 || got[0] != session.KindToolCall || got[1] != session.KindToolStart {
		t.Fatalf("账上该留着 call+start、无 result（待裁决记号）：got %v", got)
	}
}

func TestRegistryShadowingPerAgent(t *testing.T) {
	_, registry, book := newTestApp(t)

	err := registry.Register(Tool{
		Schema: chatSchema("bash", "普通 bash"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			return "普通", nil
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}
	err = registry.RegisterForScope("小红", Tool{
		Schema: chatSchema("bash", "沙箱 bash"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
			return "沙箱", nil
		},
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	app := core.New()
	t.Cleanup(app.Close)

	small := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c1", Name: "bash", Argument: json.RawMessage(`{}`), ScopeID: "小红"})
	if small.Output != "沙箱" {
		t.Fatalf("小红该拿到沙箱版：got %q", small.Output)
	}
	other := registry.ExecuteCall(context.Background(), app, book,
		Call{ID: "c2", Name: "bash", Argument: json.RawMessage(`{}`), ScopeID: "小刚"})
	if other.Output != "普通" {
		t.Fatalf("小刚该落到全局版：got %q", other.Output)
	}
}

func TestScopedChainRootOutsideChild(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	// 全局守卫挂在 root：管得住所有 agent 的调用。
	core.Intercept[Call, string](app, Guard, func(c Call, next func(Call) string) string {
		if c.Name == "echo" {
			return "全局守卫拒绝"
		}
		return next(c)
	})

	agent := app.ForChild("小红")
	result := registry.ExecuteCall(context.Background(), agent, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{}`), ScopeID: "小红"})

	if result.Status != session.ResultSkipped || result.Output != "全局守卫拒绝" {
		t.Fatalf("root 的守卫该管到子作用域：got %+v", result)
	}
}

func TestScopedChainChildLayerInsideRoot(t *testing.T) {
	app, registry, book := newTestApp(t)
	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	var order []string
	core.Intercept[PreCall, Decision](app, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		order = append(order, "root 检查")
		return next(p)
	})
	agent := app.ForChild("小红")
	core.Intercept[PreCall, Decision](agent, PreExecute, func(p PreCall, next func(PreCall) Decision) Decision {
		order = append(order, "agent 检查")
		return next(p)
	})

	registry.ExecuteCall(context.Background(), agent, book,
		Call{ID: "c1", Name: "echo", Argument: json.RawMessage(`{"Text":"好"}`), ScopeID: "小红"})

	if len(order) != 2 || order[0] != "root 检查" || order[1] != "agent 检查" {
		t.Fatalf("全局该在外层先跑，agent 定制在内层：got %v", order)
	}
}

func TestPluginRegistersInApp(t *testing.T) {
	app := core.New()
	t.Cleanup(app.Close)

	err := app.Install(Plugin{})
	if err != nil {
		t.Fatalf("装插件失败：%v", err)
	}

	_, err = Get(app)
	if err != nil {
		t.Fatalf("取登记处失败：%v", err)
	}
}

func TestSchemasRespectShadowing(t *testing.T) {
	_, registry, _ := newTestApp(t)

	err := registry.Register(echoTool())
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}
	err = registry.RegisterForScope("小红", Tool{
		Schema:  chatSchema("todo", "小红的待办"),
		Execute: func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) { return "", nil },
	})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}

	global := registry.Schemas("")
	if len(global) != 1 || global[0].Name != "echo" {
		t.Fatalf("全局列表该只有 echo：got %+v", global)
	}
	small := registry.Schemas("小红")
	if len(small) != 2 {
		t.Fatalf("小红该看见 echo+todo：got %+v", small)
	}
}

func longText(n int) string {
	return strings.Repeat("长", n)
}
