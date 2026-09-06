package subagents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
)

// 活对象。使用真实 Runner 的受控测试 Loop，不请求外部模型。
type controlledLoop struct {
	started chan loops.Invocation
	release chan struct{}
}

func (l *controlledLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "controlled test loop"}
}

func (l *controlledLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	l.started <- invocation
	if invocation.SessionID == "parent" {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.release:
	}
	// 取尽 Steer，直到最终检查点明确关闭。
	for {
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			break
		}
	}
	answer := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{
		{Kind: "reasoning", Text: "not the answer"},
		{Kind: "text", Text: "saved answer"},
	}}
	return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &answer})
}

// 数据。工具测试使用的真实服务组合。
type fixture struct {
	host     *host.Host
	registry tools.Tools
	agents   *agents.Service
	settings settings.SessionSettingsStore
	sessions *session.Store
	runner   *runner.Runner
	service  *delegation.Subagents
	loop     *controlledLoop
	parent   loops.Invocation
}

func resolve[T any](t *testing.T, h *host.Host, name string) T {
	t.Helper()
	value, err := host.Resolve[T](h, name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".harness")
	err := os.Mkdir(dataDir, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dataDir, "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	h := host.NewHost()
	t.Cleanup(func() {
		err := h.Close()
		if err != nil {
			t.Error(err)
		}
	})
	loop := &controlledLoop{started: make(chan loops.Invocation, 10), release: make(chan struct{}, 10)}
	for _, plugin := range []host.Plugin{
		&persist.Plugin{Dir: dataDir}, &session.Plugin{}, &llm.Plugin{},
		tools.NewPlugin(), events.NewPlugin(), loops.NewPlugin(), skills.NewPlugin(),
		agents.NewPlugin(), runner.NewPlugin(), delegation.NewPlugin(dataDir), New(),
	} {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
		if plugin.Name() == "loops" {
			registry := resolve[loops.Loops](t, h, "loops")
			err = registry.Register(loop)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	f := fixture{
		host: h, registry: resolve[tools.Tools](t, h, "tools"),
		agents:   resolve[*agents.Service](t, h, "agents"),
		settings: resolve[settings.SessionSettingsStore](t, h, "sessionSettings"),
		sessions: resolve[*session.Store](t, h, "sessions"),
		runner:   resolve[*runner.Runner](t, h, "runner"),
		service:  resolve[*delegation.Subagents](t, h, "subagents"), loop: loop,
	}
	_, err = f.sessions.Create("parent")
	if err != nil {
		t.Fatal(err)
	}
	err = f.settings.Put("parent", settings.SessionSettings{
		AgentID: agents.DefaultID, Model: "deepseek/deepseek-v4-flash",
		ReasoningEffort: "high", Workspace: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.runner.Start(context.Background(), "parent", session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "private parent history"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.parent = f.nextRun(t)
	return f
}

func (f fixture) nextRun(t *testing.T) loops.Invocation {
	t.Helper()
	select {
	case invocation := <-f.loop.started:
		return invocation
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
		return loops.Invocation{}
	}
}

func (f fixture) call(t *testing.T, name, args string) tools.Result {
	t.Helper()
	result, err := f.registry.Call(context.Background(), tools.Call{
		Name: name, Arguments: json.RawMessage(args), Allow: []string{name},
		SessionID: f.parent.SessionID, RunID: f.parent.RunID, ToolCallID: "call-test",
		Workspace: "/not-the-parent-workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func decode[T any](t *testing.T, result tools.Result) T {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool failed: %s", result.Content)
	}
	var value T
	err := json.Unmarshal([]byte(result.Content), &value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRegistrationAndExistingAgentPermissions(t *testing.T) {
	f := newFixture(t)
	defs := f.registry.List()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	want := []string{"subagent_list", "subagent_options", "subagent_send", "subagent_spawn", "subagent_stop", "subagent_wait"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("registered tools: %v", names)
	}
	agent, err := f.agents.Get(agents.DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.Tools) != 0 {
		t.Fatalf("plugin silently enabled tools: %v", agent.Tools)
	}
	prepared, err := f.agents.Prepare(context.Background(), agents.DefaultID, f.parent.Workspace)
	if err != nil || len(prepared.Tools) != 0 {
		t.Fatalf("disabled tools leaked: %+v, %v", prepared, err)
	}
	agent.Tools = want
	_, err = f.agents.Save(agent)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = f.agents.Prepare(context.Background(), agents.DefaultID, f.parent.Workspace)
	if err != nil || len(prepared.Tools) != 6 {
		t.Fatalf("explicit enabling failed: %+v, %v", prepared, err)
	}
	options := decode[delegation.OptionsResult](t, f.call(t, "subagent_options", "{}"))
	if len(options.Agents) == 0 || len(options.Models) == 0 {
		t.Fatalf("missing options: %+v", options)
	}
}

func TestSchemaAndCallerIdentity(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct{ name, args string }{
		{"subagent_spawn", `{"description":"job","parentSessionID":"spoof"}`},
		{"subagent_spawn", `{"description":"job","workspace":"/spoof"}`},
		{"subagent_spawn", `{"description":"   "}`},
		{"subagent_spawn", `{"description":"job","model":"unknown"}`},
		{"subagent_spawn", `{"description":"job","agentID":"unknown"}`},
		{"subagent_spawn", `{"description":"job","reasoningEffort":"unknown"}`},
		{"subagent_send", `{"taskID":"x","text":"hi","model":"other"}`},
		{"subagent_send", `{"taskID":"x","text":"   "}`},
		{"subagent_wait", `{"taskIDs":["x"],"timeoutSeconds":-1}`},
		{"subagent_wait", `{"taskIDs":["x"],"timeoutSeconds":61}`},
		{"subagent_wait", `{"taskIDs":["x"],"timeoutSeconds":0.5}`},
		{"subagent_stop", "{}"},
	} {
		result := f.call(t, tc.name, tc.args)
		if !result.IsError {
			t.Fatalf("invalid arguments accepted: %s %s", tc.name, tc.args)
		}
	}
	for _, missing := range []string{"session", "run", "call", "allow"} {
		call := tools.Call{Name: "subagent_options", Arguments: json.RawMessage("{}"),
			Allow: []string{"subagent_options"}, SessionID: "parent", RunID: f.parent.RunID, ToolCallID: "test"}
		switch missing {
		case "session":
			call.SessionID = ""
		case "run":
			call.RunID = ""
		case "call":
			call.ToolCallID = ""
		case "allow":
			call.Allow = nil
		}
		result, err := f.registry.Call(context.Background(), call)
		if err != nil || !result.IsError {
			t.Fatalf("missing %s accepted: %+v, %v", missing, result, err)
		}
	}
	tasks, err := f.service.List("parent", "")
	if err != nil || len(tasks) != 0 {
		t.Fatalf("invalid calls created tasks: %+v, %v", tasks, err)
	}
}

func TestToolLifecycleAndIsolation(t *testing.T) {
	f := newFixture(t)
	// 父设置已改动，但继承的必须仍是活 Run 的快照。
	err := f.settings.Put("parent", settings.SessionSettings{
		AgentID: "unknown", Model: "unknown", Workspace: "/changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	child := decode[delegation.SpawnResult](t, f.call(t, "subagent_spawn", `{"description":"child job"}`))
	invocation := f.nextRun(t)
	if invocation.Workspace != f.parent.Workspace || invocation.LLMConfig != f.parent.LLMConfig ||
		len(invocation.History) != 1 || invocation.History[0].Blocks[0].Text != "child job" {
		t.Fatalf("wrong inheritance or copied parent history: %+v", invocation)
	}
	taskArgs := `{"taskID":"` + child.TaskID + `"}`
	waitArgs := `{"taskIDs":["` + child.TaskID + `"]}`
	wait := decode[delegation.WaitResponse](t, f.call(t, "subagent_wait", `{"taskIDs":["`+child.TaskID+`"],"timeoutSeconds":0}`))
	if wait.Tasks[0].Status != delegation.StatusRunning {
		t.Fatalf("immediate wait: %+v", wait)
	}
	// 孩子即使有工具权限也不能派孙子。
	nested := f
	nested.parent = invocation
	if !nested.call(t, "subagent_spawn", `{"description":"grandchild"}`).IsError {
		t.Fatal("nested delegation accepted")
	}
	intruder := f
	intruder.parent.SessionID = "intruder"
	for _, name := range []string{"subagent_list", "subagent_wait", "subagent_stop"} {
		args := taskArgs
		if name == "subagent_wait" {
			args = waitArgs
		}
		if !intruder.call(t, name, args).IsError {
			t.Fatalf("%s allowed another parent's child", name)
		}
	}
	sendArgs := `{"taskID":"` + child.TaskID + `","text":"extra"}`
	if !intruder.call(t, "subagent_send", sendArgs).IsError {
		t.Fatal("send allowed another parent's child")
	}
	busy := decode[delegation.SendResult](t, f.call(t, "subagent_send", sendArgs))
	if !busy.Steered || busy.Turn != 1 || busy.RunID != child.RunID {
		t.Fatalf("busy send: %+v", busy)
	}
	f.loop.release <- struct{}{}
	wait = decode[delegation.WaitResponse](t, f.call(t, "subagent_wait", waitArgs))
	if wait.Tasks[0].Status != delegation.StatusCompleted || wait.Tasks[0].ResultEntryID == "" {
		t.Fatalf("completion: %+v", wait)
	}
	list := decode[listResult](t, f.call(t, "subagent_list", taskArgs))
	if len(list.Tasks) != 1 || len(list.Tasks[0].Results) != 1 || list.Tasks[0].Results[0].Text != "saved answer" {
		t.Fatalf("saved result query: %+v", list)
	}
	idle := decode[delegation.SendResult](t, f.call(t, "subagent_send", sendArgs))
	if idle.Steered || idle.Turn != 2 || idle.RunID == child.RunID {
		t.Fatalf("idle send: %+v", idle)
	}
	second := f.nextRun(t)
	if second.SessionID != child.ChildSessionID || second.LLMConfig != invocation.LLMConfig {
		t.Fatalf("send changed child settings: %+v", second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = f.registry.Call(ctx, tools.Call{Name: "subagent_wait", Allow: []string{"subagent_wait"},
		Arguments: json.RawMessage(waitArgs), SessionID: "parent", RunID: f.parent.RunID, ToolCallID: "wait"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait swallowed cancellation: %v", err)
	}
	stopped := decode[stopResult](t, f.call(t, "subagent_stop", taskArgs))
	if !stopped.StopRequested {
		t.Fatal("stop request missing")
	}
	wait = decode[delegation.WaitResponse](t, f.call(t, "subagent_wait", waitArgs))
	if wait.Tasks[0].Status != delegation.StatusCancelled || wait.Tasks[0].Turn != 2 {
		t.Fatalf("cancelled result: %+v", wait)
	}
	_, active := f.runner.State("parent")
	if !active {
		t.Fatal("child stop also stopped parent")
	}
}

func TestIndependentSettingOverrides(t *testing.T) {
	f := newFixture(t)
	worker, err := f.agents.Save(agents.Agent{Name: "Worker", Kind: "react", SystemPrompt: "Inspect carefully."})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args                 spawnArgs
		agent, model, effort string
	}{
		{spawnArgs{AgentID: worker.ID}, worker.ID, "deepseek/deepseek-v4-flash", "high"},
		{spawnArgs{Model: "deepseek/deepseek-v4-pro"}, agents.DefaultID, "deepseek/deepseek-v4-pro", "high"},
		{spawnArgs{ReasoningEffort: "low"}, agents.DefaultID, "deepseek/deepseek-v4-flash", "low"},
	} {
		tc.args.Description = "independent override"
		data, err := json.Marshal(tc.args)
		if err != nil {
			t.Fatal(err)
		}
		child := decode[delegation.SpawnResult](t, f.call(t, "subagent_spawn", string(data)))
		invocation := f.nextRun(t)
		setup, err := f.settings.For(child.ChildSessionID)
		if err != nil || setup.AgentID != tc.agent || setup.Model != tc.model ||
			setup.ReasoningEffort != tc.effort || setup.Workspace != f.parent.Workspace {
			t.Fatalf("settings not independently inherited: %+v, %v", setup, err)
		}
		if invocation.LLMConfig.Model != tc.model || invocation.LLMConfig.ReasoningEffort != tc.effort {
			t.Fatalf("runner received wrong settings: %+v", invocation.LLMConfig)
		}
		f.loop.release <- struct{}{}
		decode[delegation.WaitResponse](t, f.call(t, "subagent_wait", `{"taskIDs":["`+child.TaskID+`"]}`))
	}
}

func TestSendPublicationFailureDoesNotRepeatInput(t *testing.T) {
	f := newFixture(t)
	child := decode[delegation.SpawnResult](t, f.call(t, "subagent_spawn", `{"description":"child"}`))
	f.nextRun(t)
	registry := resolve[*events.Registry](t, f.host, "events")
	unsubscribe, err := events.Subscribe(registry, func(ctx context.Context, event runner.RunEvent) error {
		if event.SessionID == child.ChildSessionID && event.Kind == runner.Message && event.Entry.Message.Role == session.RoleUser {
			return errors.New("publication failed after durable acceptance")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	result := f.call(t, "subagent_send", `{"taskID":"`+child.TaskID+`","text":"exactly once"}`)
	if !result.IsError {
		t.Fatal("send hid publication failure")
	}
	wait := decode[delegation.WaitResponse](t, f.call(t, "subagent_wait", `{"taskIDs":["`+child.TaskID+`"]}`))
	if wait.Tasks[0].Turn != 1 || wait.Tasks[0].Status != delegation.StatusCancelled {
		t.Fatalf("send started a replacement run after ambiguous error: %+v", wait)
	}
	sess, err := f.sessions.Get(child.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range sess.Entries() {
		for _, block := range entry.Message.Blocks {
			if block.Text == "exactly once" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("input appended %d times", count)
	}
}

func TestListKeepsRecordsWhenPersistenceFails(t *testing.T) {
	f := newFixture(t)
	child := decode[delegation.SpawnResult](t, f.call(t, "subagent_spawn", `{"description":"child"}`))
	f.nextRun(t)
	// HOME 由 fixture 指向测试临时目录，不触及用户数据。
	path := filepath.Join(os.Getenv("HOME"), ".harness", "subagents", "tasks", child.TaskID+".json.tmp")
	err := os.Mkdir(path, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	f.loop.release <- struct{}{}
	if !f.call(t, "subagent_wait", `{"taskIDs":["`+child.TaskID+`"]}`).IsError {
		t.Fatal("wait hid persistence failure")
	}
	result := f.call(t, "subagent_list", "{}")
	var value listResult
	err = json.Unmarshal([]byte(result.Content), &value)
	if err != nil || !result.IsError || value.Error == "" || len(value.Tasks) != 1 || value.Tasks[0].ID != child.TaskID {
		t.Fatalf("list hid record or error: %+v, %v", result, err)
	}
	err = f.host.Close()
	if err == nil {
		t.Fatal("close hid persistence failure")
	}
}
