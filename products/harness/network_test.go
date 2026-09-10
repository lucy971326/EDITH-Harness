package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"harness/appserver"
	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/skills"
	"harness/kernel/subagents"
	"harness/kernel/tools"
	"harness/plugins/kernel/loops/react"
)

// 全链路使用真正的 ReAct / Runner / Product，只有模型 HTTP 服务是本地替身。
func TestTypeScriptClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires Node.js 22.18+ for the TypeScript test client")
	}
	model := &networkModel{cancelled: make(chan struct{}, 1)}
	modelServer := httptest.NewServer(model)
	t.Cleanup(modelServer.Close)
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)
	data := filepath.Join(testHome, ".harness")
	err = os.Mkdir(data, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(data, "config.yaml"), []byte(fmt.Sprintf("providers:\n  deepseek:\n    apiKey: test-key\n    baseURL: %s\n", modelServer.URL)), 0600)
	if err != nil {
		t.Fatal(err)
	}
	h := host.NewHost()
	server := appserver.New()
	t.Cleanup(func() { _ = h.Close() })
	t.Cleanup(func() { _ = server.Close() })
	err = h.RegisterService("appServer", server)
	if err != nil {
		t.Fatal(err)
	}
	plugins := []host.Plugin{&persist.Plugin{Dir: data}, &session.Plugin{}, &llm.Plugin{}, events.NewPlugin(), tools.NewPlugin(), loops.NewPlugin(), react.New(), skills.NewPlugin(), agents.NewPlugin(), commands.NewPlugin(), runner.NewPlugin(), subagents.NewPlugin(data), NewPlugin()}
	for _, plugin := range plugins {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(plugin.Name(), err)
		}
	}
	url, err := server.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../clients/test/smoke.ts")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, script)
	command.Env = append(os.Environ(), "HARNESS_TEST_RPC_URL="+url, "HARNESS_TEST_WORKSPACE="+t.TempDir())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("TypeScript client: %v\n%s", err, output)
	}
	t.Log(string(output))
	select {
	case <-model.cancelled:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the model HTTP request")
	}
}

type networkModel struct{ cancelled chan struct{} }

func (m *networkModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hold := false
	for _, message := range request.Messages {
		if message.Role == "user" && string(message.Content) == `"hold"` {
			hold = true
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if hold {
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"waiting\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		m.cancelled <- struct{}{}
		return
	}
	fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"local model completed\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
}
