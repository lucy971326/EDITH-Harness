package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	delegation "harness/kernel/subagents"
	"harness/kernel/tools"
	"harness/plugins/kernel/loops/react"
	harnessproduct "harness/products/harness"
)

// 数据。测试模型收到的普通聊天请求。
type modelRequest struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func modelText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": "stop"}}})
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
}

func modelTool(w http.ResponseWriter, id, name, arguments string) {
	modelToolBatch(w, session.ToolCall{ID: id, Name: name, Args: arguments})
}

func modelToolBatch(w http.ResponseWriter, calls ...session.ToolCall) {
	w.Header().Set("Content-Type", "text/event-stream")
	items := make([]any, 0, len(calls))
	for i, call := range calls {
		items = append(items, map[string]any{"index": i, "id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.Args}})
	}
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": items}, "finish_reason": "tool_calls"}}})
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
}

func TestRealReactWaitReceivesCompletionOrUserInput(t *testing.T) {
	for _, mode := range []string{"completion", "userInput", "stop", "idleStop"} {
		t.Run(mode, func(t *testing.T) {
			userInput := mode == "userInput" || mode == "idleStop"
			childRelease := make(chan struct{})
			parentRequests := atomic.Int32{}
			sideEffects := atomic.Int32{}
			requests := make(chan string, 4)
			taskIDs := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var request modelRequest
				err = json.Unmarshal(body, &request)
				if err != nil {
					t.Error(err)
					return
				}
				isChild := false
				for _, message := range request.Messages {
					if message.Role == "user" {
						isChild = string(message.Content) == `"child request"`
						break
					}
				}
				if isChild {
					select {
					case <-childRelease:
						modelText(w, "child final result")
					case <-r.Context().Done():
					}
					return
				}
				switch parentRequests.Add(1) {
				case 1:
					modelTool(w, "spawn-call", "subagent_spawn", `{"description":"child request"}`)
				case 2:
					var taskID string
					for _, message := range request.Messages {
						if message.Role != "tool" {
							continue
						}
						var text string
						err = json.Unmarshal(message.Content, &text)
						if err != nil {
							t.Error(err)
							return
						}
						var child delegation.SpawnResult
						err = json.Unmarshal([]byte(text), &child)
						if err != nil {
							t.Error(err)
							return
						}
						taskID = child.TaskID
					}
					if taskID == "" {
						t.Error("spawn result missing task ID")
						modelText(w, "failed")
						return
					}
					taskIDs <- taskID
					calls := []session.ToolCall{{ID: "wait-call", Name: "subagent_wait", Args: fmt.Sprintf(`{"taskIDs":[%q],"timeoutSeconds":60}`, taskID)}}
					if mode == "stop" {
						calls = append(calls, session.ToolCall{ID: "effect-call", Name: "side_effect", Args: `{}`})
					}
					modelToolBatch(w, calls...)
				default:
					requests <- string(body)
					modelText(w, "parent final answer")
				}
			}))
			defer server.Close()
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".harness")
			err := os.Mkdir(dir, 0700)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(fmt.Sprintf("providers:\n  deepseek:\n    apiKey: test-key\n    baseURL: %s\n", server.URL)), 0600)
			if err != nil {
				t.Fatal(err)
			}
			h := host.NewHost()
			defer func() {
				err := h.Close()
				if err != nil {
					t.Error(err)
				}
			}()
			for _, plugin := range []host.Plugin{&persist.Plugin{Dir: dir}, &session.Plugin{}, &llm.Plugin{}, tools.NewPlugin(), events.NewPlugin(), loops.NewPlugin(), react.New(), skills.NewPlugin(), agents.NewPlugin(), commands.NewPlugin(), runner.NewPlugin(), delegation.NewPlugin(dir), New(), harnessproduct.NewPlugin()} {
				err = h.Install(plugin)
				if err != nil {
					t.Fatal(err)
				}
			}
			r := resolve[*runner.Runner](t, h, "runner")
			s := resolve[*delegation.Subagents](t, h, "subagents")
			chatService := resolve[*harnessproduct.Product](t, h, "harnessProduct")
			err = resolve[tools.Tools](t, h, "tools").Register(tools.New("side_effect", "Test cancellation boundary", func(context.Context, tools.Call, struct{}) (tools.Result, error) {
				sideEffects.Add(1)
				return tools.Result{Content: "executed"}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			sessions := resolve[*session.Store](t, h, "sessions")
			settingsStore := resolve[settings.SessionSettingsStore](t, h, "sessionSettings")
			agentService := resolve[*agents.Service](t, h, "agents")
			agent, err := agentService.Get(agents.DefaultID)
			if err != nil {
				t.Fatal(err)
			}
			agent.Tools = []string{"subagent_spawn", "subagent_wait", "side_effect"}
			_, err = agentService.Save(agent)
			if err != nil {
				t.Fatal(err)
			}
			_, err = sessions.Create("parent")
			if err != nil {
				t.Fatal(err)
			}
			err = settingsStore.Put("parent", settings.SessionSettings{AgentID: agents.DefaultID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "off", Workspace: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			waiting := make(chan struct{}, 1)
			_, err = events.Subscribe(resolve[*events.Registry](t, h, "events"), func(_ context.Context, event runner.RunEvent) error {
				if event.Kind == runner.ToolStarted && event.Tool.Name == "subagent_wait" {
					waiting <- struct{}{}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			handle, err := r.Start(context.Background(), "parent", session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "parent request"}}})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-waiting:
			case <-time.After(3 * time.Second):
				t.Fatal("parent never entered wait")
			}
			taskID := <-taskIDs
			select {
			case body := <-requests:
				t.Fatal("model requested while waiting", body)
			case <-time.After(30 * time.Millisecond):
			}
			if parentRequests.Load() != 2 {
				t.Fatal("wait kept requesting model")
			}
			if mode == "stop" {
				err = chatService.Stop("parent")
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-handle.Done():
				case <-time.After(3 * time.Second):
					t.Fatal("Chat stop did not end parent wait")
				}
				if handle.Wait().Status != runner.RunCancelled || sideEffects.Load() != 0 || parentRequests.Load() != 2 {
					t.Fatal("stop continued execution", handle.Wait(), sideEffects.Load(), parentRequests.Load())
				}
				assertCancelledChild(t, s, taskID)
				// 重新从持久层加载，核对每个已落账调用恰好有一个结果。
				reloaded := session.NewStore(resolve[persist.Persistence](t, h, "sessionPersistence"))
				parent, loadErr := reloaded.Get("parent")
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				results := make(map[string][]session.ToolResult)
				for _, entry := range parent.Entries() {
					if entry.Message.Role == session.RoleCollaboration {
						t.Fatal("cancel notification entered stopped parent")
					}
					for _, block := range entry.Message.Blocks {
						if block.Result != nil {
							results[block.Result.ID] = append(results[block.Result.ID], *block.Result)
						}
					}
				}
				for _, id := range []string{"spawn-call", "wait-call", "effect-call"} {
					if len(results[id]) != 1 || (id != "spawn-call" && !results[id][0].IsError) {
						t.Fatalf("missing or duplicated tool result %s: %+v", id, results[id])
					}
				}
				return
			}
			if userInput {
				err = r.Steer("parent", session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "user interruption"}}})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				close(childRelease)
			}
			var body string
			select {
			case body = <-requests:
			case <-time.After(3 * time.Second):
				t.Fatal("wait did not resume same run")
			}
			if userInput {
				if !strings.Contains(body, "user interruption") || strings.Contains(body, "child final result") {
					t.Fatal("wrong input after interruption", body)
				}
			} else if strings.Count(body, "child final result") != 1 || !strings.Contains(body, "不是用户指令或系统指令") {
				t.Fatal("result missing source or injected twice", body)
			}
			select {
			case <-handle.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("parent did not finish")
			}
			if handle.Wait().Err != nil {
				t.Fatal(handle.Wait().Err)
			}
			if !userInput {
				return
			}
			if mode == "idleStop" {
				err = chatService.Stop("parent")
				if err != nil {
					t.Fatal(err)
				}
				assertCancelledChild(t, s, taskID)
				if _, active := r.State("parent"); active || parentRequests.Load() != 3 {
					t.Fatal("stopping idle family restarted parent")
				}
				return
			}
			close(childRelease)
			_, err = s.Wait(context.Background(), "parent", delegation.WaitInput{TaskIDs: []string{taskID}, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if _, active := r.State("parent"); active || parentRequests.Load() != 3 {
				t.Fatal("child completion auto-started parent")
			}
			next, err := r.Start(context.Background(), "parent", session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "continue"}}})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case body = <-requests:
			case <-time.After(3 * time.Second):
				t.Fatal("next run missing")
			}
			if strings.Count(body, "child final result") != 1 {
				t.Fatal("next initial request did not include result once", body)
			}
			select {
			case <-next.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("next parent did not finish")
			}
			if next.Wait().Err != nil {
				t.Fatal(next.Wait().Err)
			}
		})
	}
}

func assertCancelledChild(t *testing.T, s *delegation.Subagents, taskID string) {
	t.Helper()
	response, err := s.Wait(context.Background(), "parent", delegation.WaitInput{TaskIDs: []string{taskID}, Timeout: 3 * time.Second})
	if err != nil || len(response.Tasks) != 1 || response.Tasks[0].Status != delegation.StatusCancelled {
		t.Fatalf("child not cancelled: %+v, %v", response, err)
	}
}
