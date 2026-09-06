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
	w.Header().Set("Content-Type", "text/event-stream")
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}}}, "finish_reason": "tool_calls"}}})
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
}

func TestRealReactWaitReceivesCompletionOrUserInput(t *testing.T) {
	for _, userInput := range []bool{false, true} {
		t.Run(fmt.Sprintf("userInput=%v", userInput), func(t *testing.T) {
			childRelease := make(chan struct{})
			parentRequests := atomic.Int32{}
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
					modelTool(w, "wait-call", "subagent_wait", fmt.Sprintf(`{"taskIDs":[%q],"timeoutSeconds":60}`, taskID))
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
			for _, plugin := range []host.Plugin{&persist.Plugin{Dir: dir}, &session.Plugin{}, &llm.Plugin{}, tools.NewPlugin(), events.NewPlugin(), loops.NewPlugin(), react.New(), skills.NewPlugin(), agents.NewPlugin(), runner.NewPlugin(), delegation.NewPlugin(dir), New()} {
				err = h.Install(plugin)
				if err != nil {
					t.Fatal(err)
				}
			}
			r := resolve[*runner.Runner](t, h, "runner")
			s := resolve[*delegation.Subagents](t, h, "subagents")
			sessions := resolve[*session.Store](t, h, "sessions")
			settingsStore := resolve[settings.SessionSettingsStore](t, h, "sessionSettings")
			agentService := resolve[*agents.Service](t, h, "agents")
			agent, err := agentService.Get(agents.DefaultID)
			if err != nil {
				t.Fatal(err)
			}
			agent.Tools = []string{"subagent_spawn", "subagent_wait"}
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
