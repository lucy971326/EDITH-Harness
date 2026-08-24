package loop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/persistence/jsonl"
	"harness/persistence/profilejson"
	"harness/session"
	"harness/tools"
)

const (
	crashAgentID   = "崩溃小红"
	crashSessionID = "崩溃会话"
)

// crashAdapter 是跨进程恢复测试的确定性模型；它只记录新进程有没有真的重发请求。
type crashAdapter struct {
	mu       sync.Mutex
	requests []llm.Request
}

func (a *crashAdapter) Name() string {
	return "crash-test"
}

func (a *crashAdapter) Stream(_ context.Context, request llm.Request, onDelta func(chat.Delta)) (llm.Reply, error) {
	a.mu.Lock()
	a.requests = append(a.requests, request)
	a.mu.Unlock()
	if onDelta != nil {
		onDelta(chat.Delta{Text: "恢复后已处理"})
	}
	return llm.Reply{Text: "恢复后已处理", StopReason: "stop"}, nil
}

func (a *crashAdapter) requestCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

// TestCrashRecoveryHelper 只由父测试启动；它写到指定提交点后 SIGKILL 自己，绝不执行 Close。
func TestCrashRecoveryHelper(t *testing.T) {
	scenario := os.Getenv("HARNESS_CRASH_SCENARIO")
	if scenario == "" {
		return
	}
	root := os.Getenv("HARNESS_CRASH_ROOT")
	if root == "" {
		t.Fatal("崩溃测试缺少 HARNESS_CRASH_ROOT")
	}

	app, roster, _ := newCrashStack(t, root)
	_ = app
	err := roster.CreateAgent(agents.AgentProfile{
		ID:       crashAgentID,
		Provider: "crash-test",
		Model:    "test-model",
		Thinking: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := roster.StartSession(crashAgentID, crashSessionID)
	if err != nil {
		t.Fatal(err)
	}
	book := conversation.Book()

	switch scenario {
	case "delivery":
		_, err = book.RecordDeliver("d1", "重启前还没领出的消息", TargetNextTurn)
	case "model-window":
		_, err = book.RecordUserMessage("模型可能已经收到的消息")
		if err == nil {
			_, err = book.RecordTurnStart()
		}
		if err == nil {
			_, err = book.RecordStepStart()
		}
		if err == nil {
			_, err = book.RecordSnapshot([]byte(`{}`))
		}
	case "unstarted-tool":
		err = recordCrashToolCall(book, false)
	case "started-tool":
		err = recordCrashToolCall(book, true)
	case "torn-tail":
		_, err = book.RecordUserMessage("完整的一笔")
		if err == nil {
			err = appendTornTail(root)
		}
	default:
		t.Fatalf("不认识的崩溃场景：%s", scenario)
	}
	if err != nil {
		t.Fatal(err)
	}
	err = book.Flush()
	if err != nil {
		t.Fatal(err)
	}

	err = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCrashRecoveryAcrossRealJSONL(t *testing.T) {
	cases := []struct {
		name   string
		verify func(*testing.T, agents.Service, *crashAdapter, string)
	}{
		{
			name: "delivery",
			verify: func(t *testing.T, roster agents.Service, adapter *crashAdapter, root string) {
				conversation, err := roster.ResumeSession(crashSessionID)
				if err != nil {
					t.Fatal(err)
				}
				conversation.WaitIdle()
				if adapter.requestCount() != 1 {
					t.Fatalf("未领消息重启后应只请求模型一次，实际 %d 次", adapter.requestCount())
				}
				if !historyContains(conversation.Book().ModelHistory(), "重启前还没领出的消息") {
					t.Fatalf("未领消息在重启后丢了：%+v", conversation.Book().ModelHistory())
				}
			},
		},
		{
			name: "model-window",
			verify: func(t *testing.T, roster agents.Service, adapter *crashAdapter, root string) {
				conversation, err := roster.ResumeSession(crashSessionID)
				if err != nil {
					t.Fatal(err)
				}
				conversation.WaitIdle()
				if adapter.requestCount() != 0 {
					t.Fatalf("模型请求窗口重启后不许自动重发，实际请求 %d 次", adapter.requestCount())
				}
				joined := strings.Join(kinds(conversation.Book().Events()), ",")
				if !strings.Contains(joined, session.KindStepEnd) || !strings.Contains(joined, session.KindTurnEnd) {
					t.Fatalf("模型请求窗口的轮步应收口：%s", joined)
				}
			},
		},
		{
			name: "unstarted-tool",
			verify: func(t *testing.T, roster agents.Service, adapter *crashAdapter, root string) {
				conversation, err := roster.ResumeSession(crashSessionID)
				if err != nil {
					t.Fatal(err)
				}
				assertCrashToolResult(t, conversation.Book().Events(), session.ResultSkipped)
				if adapter.requestCount() != 0 {
					t.Fatal("恢复未开跑工具时不该自动问模型")
				}
			},
		},
		{
			name: "started-tool",
			verify: func(t *testing.T, roster agents.Service, adapter *crashAdapter, root string) {
				conversation, err := roster.ResumeSession(crashSessionID)
				if err != nil {
					t.Fatal(err)
				}
				assertCrashToolResult(t, conversation.Book().Events(), tools.ResultUnknown)
				history := conversation.Book().ModelHistory()
				last := history[len(history)-1]
				if last.Text != "半句话" || !last.Interrupted {
					t.Fatalf("流式半句应固化为被打断的定稿：%+v", last)
				}
				if adapter.requestCount() != 0 {
					t.Fatal("结果不明的工具不许自动重跑或自动问模型")
				}
			},
		},
		{
			name: "torn-tail",
			verify: func(t *testing.T, roster agents.Service, adapter *crashAdapter, root string) {
				conversation, err := roster.ResumeSession(crashSessionID)
				if err != nil {
					t.Fatal(err)
				}
				if !historyContains(conversation.Book().ModelHistory(), "完整的一笔") {
					t.Fatalf("修尾后完整事件不该丢：%+v", conversation.Book().ModelHistory())
				}
				data, err := os.ReadFile(crashBookPath(root))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(string(data), "\n") || strings.Contains(string(data), `{"kind":`) {
					t.Fatalf("重启只该删最后半行：%q", data)
				}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runCrashHelper(t, root, test.name)
			app, roster, adapter := newCrashStack(t, root)
			t.Cleanup(func() {
				app.Close()
			})
			test.verify(t, roster, adapter, root)
		})
	}
}

func newCrashStack(t *testing.T, root string) (*core.App, agents.Service, *crashAdapter) {
	t.Helper()
	app := core.New()
	err := app.Install(
		profilejson.Plugin{Root: filepath.Join(root, "agents")},
		jsonl.Plugin{Root: filepath.Join(root, "sessions")},
		session.Plugin{},
		llm.Plugin{},
		tools.Plugin{},
		agents.Plugin{},
		Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &crashAdapter{}
	models, err := llm.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = models.Register(adapter)
	if err != nil {
		t.Fatal(err)
	}
	roster, err := agents.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	return app, roster, adapter
}

func runCrashHelper(t *testing.T, root string, scenario string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestCrashRecoveryHelper$")
	command.Env = append(os.Environ(),
		"HARNESS_CRASH_ROOT="+root,
		"HARNESS_CRASH_SCENARIO="+scenario,
	)
	err := command.Run()
	if err == nil {
		t.Fatal("崩溃子进程不该正常退出")
	}
	exitError, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("启动崩溃子进程失败：%v", err)
	}
	status, ok := exitError.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || status.Signal() != syscall.SIGKILL {
		t.Fatalf("子进程必须被 SIGKILL 杀死，实际：%v", err)
	}
}

func recordCrashToolCall(book *session.Session, started bool) error {
	_, err := book.RecordTurnStart()
	if err != nil {
		return err
	}
	_, err = book.RecordStepStart()
	if err != nil {
		return err
	}
	_, err = book.RecordToolCall(session.ToolCallData{ID: "call-1", Name: "danger"})
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	_, err = book.RecordToolStart("call-1")
	if err != nil {
		return err
	}
	_, err = book.RecordChunk("半句话")
	return err
}

func appendTornTail(root string) error {
	file, err := os.OpenFile(crashBookPath(root), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, err = file.WriteString(`{"kind":`)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func crashBookPath(root string) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(crashSessionID))
	return filepath.Join(root, "sessions", name+".jsonl")
}

func assertCrashToolResult(t *testing.T, events []session.Event, status string) {
	t.Helper()
	for _, event := range events {
		if event.Kind != session.KindToolResult {
			continue
		}
		var result session.ToolResultData
		err := json.Unmarshal(event.Data, &result)
		if err != nil {
			t.Fatal(err)
		}
		if result.CallID == "call-1" && result.Status == status {
			return
		}
	}
	t.Fatalf("没有找到 call-1 的 %s 结果：%+v", status, events)
}

func historyContains(history []chat.Message, text string) bool {
	for _, message := range history {
		if message.Text == text {
			return true
		}
	}
	return false
}
