package conversations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"harness/appserver"
	"harness/kernel/agents"
	"harness/kernel/approvals"
	"harness/kernel/commands"
	"harness/kernel/conversations"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/subagents"
	"harness/kernel/tools"
	compactcmd "harness/plugins/commands/compact"
	"harness/plugins/loops/react"
	machinelocal "harness/plugins/machine/local"
	skillsbuiltin "harness/plugins/skills/builtin"
	skillsfilesystem "harness/plugins/skills/filesystem"
)

// 全链路使用真正的 ReAct / Runner / conversations.Service，只有模型 HTTP 服务是本地替身。
func TestTypeScriptClient(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires Node.js 22.18+ for the TypeScript test client")
	}
	manual := os.Getenv("HARNESS_WEB_QA") == "1"
	model := &networkModel{cancelled: make(chan struct{}, 8), release: make(chan struct{}, 1), restart: make(chan struct{}, 1)}
	modelServer := httptest.NewServer(model)
	t.Cleanup(modelServer.Close)
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)
	data := filepath.Join(testHome, ".harness")
	err = os.Mkdir(data, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(data, "config.yaml"), []byte(fmt.Sprintf("providers:\n  deepseek:\n    apiKey: test-key\n    baseURL: %s\n  google:\n    apiKey: test-key\n    baseURL: %s\n", modelServer.URL, modelServer.URL)), 0600)
	if err != nil {
		t.Fatal(err)
	}
	h, server, product := newNetworkHost(t, data)
	address := "127.0.0.1:0"
	if manual {
		address = "127.0.0.1:8888"
	}
	webURL, err := server.Listen(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	url := "ws" + strings.TrimPrefix(webURL, "http") + "/rpc"
	if manual {
		workspace := filepath.Join(testHome, "浏览器验收")
		err = os.Mkdir(workspace, 0700)
		if err != nil {
			t.Fatal(err)
		}
		_, err = product.Create(workspace)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("隔离后台 %s；测试控制 %s/qa/release、/qa/restart；输入 hold 等待、普通文字立即回答", url, modelServer.URL)
		ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-model.restart:
				err = server.Close()
				if err != nil {
					t.Fatal(err)
				}
				err = h()
				if err != nil {
					t.Fatal(err)
				}
				h, server, product = newNetworkHost(t, data)
				_, err = server.Listen(address, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Log("已重新创建 Host / Runner / appserver，沿用隔离账本")
			}
		}
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
	webScript, err := filepath.Abs("../../clients/web/test/network.ts")
	if err != nil {
		t.Fatal(err)
	}
	webCommand := exec.CommandContext(ctx, node, "--experimental-strip-types", webScript)
	webCommand.Env = append(os.Environ(), "HARNESS_TEST_RPC_URL="+url, "HARNESS_TEST_WORKSPACE="+t.TempDir())
	output, err = webCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("正式 Web Client: %v\n%s", err, output)
	}
	t.Log(string(output))
	select {
	case <-model.cancelled:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the model HTTP request")
	}
}

// 真实公共服务组装可重启；每次都重新创建 Runner，不能靠旧内存通过恢复验收。
func newNetworkHost(t *testing.T, data string) (func() error, *appserver.Server, *conversations.Service) {
	t.Helper()
	files, err := persist.NewFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	disk := session.NewPersistence(files)
	settingsStore := settings.NewStore(files)
	sessions := session.NewStore(disk)
	models, err := llm.New(files)
	if err != nil {
		t.Fatal(err)
	}
	machineService, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = machineService.Close() })
	eventRegistry := events.NewRegistry()
	loopRegistry := loops.NewRegistry()
	toolRegistry := tools.NewRegistry()
	skillService := skills.NewRegistry()
	err = loopRegistry.Register(react.New(models, toolRegistry))
	if err != nil {
		t.Fatal(err)
	}
	agentService, err := agents.NewService(agents.NewStore(files), settingsStore, loopRegistry, toolRegistry, skillService)
	if err != nil {
		t.Fatal(err)
	}
	runService, err := runner.NewRunner(sessions, settingsStore, agentService, loopRegistry, eventRegistry, models, toolRegistry, files, machineService)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runService.Close)
	subFiles, err := files.Scope("subagents")
	if err != nil {
		t.Fatal(err)
	}
	subagentService, err := subagents.NewSubagents(sessions, settingsStore, agentService, models, runService, eventRegistry, subFiles)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subagentService.Close() })
	closeServices := func() error {
		err := subagentService.Close()
		runService.Close()
		return errors.Join(err, machineService.Close())
	}
	commandService := commands.NewRegistry()
	approvalService, err := approvals.Open(files, models, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = approvalService.Close() })
	service, err := conversations.New(sessions, settingsStore, agentService, models, runService, commandService, subagentService, approvalService)
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := skillsbuiltin.New(files)
	if err != nil {
		t.Fatal(err)
	}
	err = skillService.Register(builtin)
	if err != nil {
		t.Fatal(err)
	}
	err = skillService.Register(skillsfilesystem.New(machineService, files))
	if err != nil {
		t.Fatal(err)
	}
	err = commandService.Register(compactcmd.New(runService))
	if err != nil {
		t.Fatal(err)
	}
	server := appserver.New()
	t.Cleanup(func() { _ = server.Close() })
	err = server.BindHarness(service, runService, eventRegistry)
	if err != nil {
		t.Fatal(err)
	}
	err = server.BindModels(models)
	if err != nil {
		t.Fatal(err)
	}
	err = server.BindAgents(agentService)
	if err != nil {
		t.Fatal(err)
	}
	err = server.BindSkills(skillService)
	if err != nil {
		t.Fatal(err)
	}
	err = server.BindCommands(commandService)
	if err != nil {
		t.Fatal(err)
	}
	return closeServices, server, service

}

type networkModel struct {
	cancelled chan struct{}
	release   chan struct{}
	restart   chan struct{}
}

func (m *networkModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/qa/restart" {
		select {
		case m.restart <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if r.URL.Path == "/qa/release" {
		select {
		case m.release <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
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
		if message.Role == "user" {
			hold = string(message.Content) == `"hold"`
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if hold {
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"waiting\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			select {
			case m.cancelled <- struct{}{}:
			default:
			}
		case <-m.release:
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" 已继续\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":21,\"completion_tokens\":2,\"total_tokens\":23,\"prompt_tokens_details\":{\"cached_tokens\":8}}}\n\ndata: [DONE]\n\n")
		}
		return
	}
	fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"local model completed\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":21,\"completion_tokens\":2,\"total_tokens\":23,\"prompt_tokens_details\":{\"cached_tokens\":8}}}\n\ndata: [DONE]\n\n")
}
