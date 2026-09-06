package subagents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
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
	"harness/kernel/tools"
)

// 数据。测试脚手架。
type subagentsFixture struct {
	host      *host.Host
	subagents *Subagents
	runner    *runner.Runner
	sessions  *session.Store
	settings  settings.SessionSettingsStore
	loop      *testLoop
	dataDir   string
}

func newSubagentsFixture(t *testing.T) subagentsFixture {
	t.Helper()
	home := t.TempDir()
	dataDir := filepath.Join(home, ".harness")
	err := os.Mkdir(dataDir, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dataDir, "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	prevHome := os.Getenv("HOME")
	err = os.Setenv("HOME", home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", prevHome) })

	h := host.NewHost()
	plugins := []host.Plugin{
		&persist.Plugin{Dir: t.TempDir()},
		&session.Plugin{},
		&llm.Plugin{},
		events.NewPlugin(),
		loops.NewPlugin(),
		skills.NewPlugin(),
		tools.NewPlugin(),
		agents.NewPlugin(),
		commands.NewPlugin(),
		runner.NewPlugin(),
		NewPlugin(dataDir),
	}
	loop := newTestLoop()

	for _, plugin := range plugins {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
		if plugin.Name() == "loops" {
			registry, err := host.Resolve[loops.Loops](h, "loops")
			if err != nil {
				t.Fatal(err)
			}
			err = registry.Register(loop)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	subService, err := host.Resolve[*Subagents](h, "subagents")
	if err != nil {
		t.Fatal(err)
	}
	runSvc, err := host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		t.Fatal(err)
	}
	sessStore, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}

	return subagentsFixture{
		host:      h,
		subagents: subService,
		runner:    runSvc,
		sessions:  sessStore,
		settings:  settingsStore,
		loop:      loop,
		dataDir:   dataDir,
	}
}

// 活对象。测试用 Loop 实现。
type testLoop struct {
	started         chan struct{}
	releaseCh       chan struct{}
	parentStarted   chan struct{}
	parentReleaseCh chan struct{}
}

func newTestLoop() *testLoop {
	return &testLoop{
		started:         make(chan struct{}, 32),
		releaseCh:       make(chan struct{}, 32),
		parentStarted:   make(chan struct{}, 32),
		parentReleaseCh: make(chan struct{}, 32),
	}
}

func (l *testLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "test react loop"}
}

func (l *testLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	if invocation.SessionID == "parent-session" {
		l.parentStarted <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.parentReleaseCh:
			return nil
		}
	}

	l.started <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.releaseCh:
	}

	steers, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
	if err != nil {
		return err
	}

	text := "child assistant answer"
	if len(steers) > 0 {
		text = "child answer with steer"
	}
	msg := session.Message{
		Role:   session.RoleAssistant,
		Blocks: []session.Block{{Kind: "text", Text: text}},
	}
	return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &msg})
}

func (l *testLoop) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-l.started:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not start within deadline")
	}
}

func (l *testLoop) waitParentStarted(t *testing.T) {
	t.Helper()
	select {
	case <-l.parentStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("parent loop did not start within deadline")
	}
}

func (l *testLoop) release() {
	l.releaseCh <- struct{}{}
}

func (l *testLoop) releaseParent() {
	select {
	case l.parentReleaseCh <- struct{}{}:
	default:
	}
}

// 活对象。测试用拦截 Session 设置操作的存储包装器。
type barrierSettingsStore struct {
	settings.SessionSettingsStore
	beforePut func(sessionID string)
}

func (b *barrierSettingsStore) Put(sessionID string, s settings.SessionSettings) error {
	if b.beforePut != nil {
		b.beforePut(sessionID)
	}
	return b.SessionSettingsStore.Put(sessionID, s)
}

func createParentRun(t *testing.T, f subagentsFixture, workspace string) (string, string) {
	t.Helper()
	_, err := f.sessions.Create("parent-session")
	if err != nil {
		t.Fatal(err)
	}
	err = f.settings.Put("parent-session", settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "deepseek/deepseek-v4-flash",
		ReasoningEffort: "high",
		Workspace:       workspace,
	})
	if err != nil {
		t.Fatal(err)
	}

	handle, err := f.runner.Start(context.Background(), "parent-session", session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "parent user prompt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitParentStarted(t)
	return "parent-session", handle.RunID()
}

func TestStoreAtomicAndRecovery(t *testing.T) {
	dir := t.TempDir()
	store, err := newTaskStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	task := Task{
		Version:         1,
		ID:              "task-1",
		Turns:           []TurnRecord{{Turn: 1, Status: StatusPending}},
		ParentSessionID: "parent-1",
		ChildSessionID:  "child-1",
		Description:     "test task",
		Status:          StatusPending,
		Turn:            1,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	err = store.saveTask(task)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := store.loadTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != task.ID || loaded.Status != StatusPending {
		t.Fatalf("loaded task mismatch: %+v", loaded)
	}

	// 损坏文件测试：listTasks 必须报错，不能静默丢弃
	corruptPath := filepath.Join(dir, "tasks", "corrupted.json")
	err = os.WriteFile(corruptPath, []byte("invalid json"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.listTasks()
	if err == nil {
		t.Fatal("expected error on corrupted task file, got nil")
	}
	_ = os.Remove(corruptPath)

	// 未知版本测试：必须报错
	unknownVersionTask := filepath.Join(dir, "tasks", "unknown.json")
	err = os.WriteFile(unknownVersionTask, []byte(`{"version": 999, "id": "unknown", "parentSessionID": "p", "childSessionID": "c", "status": "pending", "turn": 1}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.listTasks()
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
	_, err = store.loadTask("unknown")
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion on loadTask, got %v", err)
	}
	_ = os.Remove(unknownVersionTask)
}

func TestStoreValidationAndIntegrity(t *testing.T) {
	dir := t.TempDir()
	store, err := newTaskStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	validTask := Task{
		Version:         1,
		ID:              "task-val-1",
		Turns:           []TurnRecord{{Turn: 1, Status: StatusPending}},
		ParentSessionID: "parent-val-1",
		ChildSessionID:  "child-val-1",
		Description:     "valid task",
		Status:          StatusPending,
		Turn:            1,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	// 1. Version 必须 == 1，不能为 0 或大于 1
	v0Task := validTask
	v0Task.Version = 0
	err = store.saveTask(v0Task)
	if err == nil {
		t.Fatal("expected error for task version 0, got nil")
	}

	v2Task := validTask
	v2Task.Version = 2
	err = store.saveTask(v2Task)
	if err == nil {
		t.Fatal("expected error for task version 2, got nil")
	}

	// 2. ID 不能为空
	noIDTask := validTask
	noIDTask.ID = ""
	err = store.saveTask(noIDTask)
	if err == nil {
		t.Fatal("expected error for empty task ID, got nil")
	}

	// 3. ParentSessionID 不能为空
	noParentTask := validTask
	noParentTask.ParentSessionID = ""
	err = store.saveTask(noParentTask)
	if err == nil {
		t.Fatal("expected error for empty ParentSessionID, got nil")
	}

	// 4. ChildSessionID 不能为空
	noChildTask := validTask
	noChildTask.ChildSessionID = ""
	err = store.saveTask(noChildTask)
	if err == nil {
		t.Fatal("expected error for empty ChildSessionID, got nil")
	}

	// 5. 状态必须合规
	badStatusTask := validTask
	badStatusTask.Status = "invalid_status"
	err = store.saveTask(badStatusTask)
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}

	// 6. Turn 必须 >= 1
	turn0Task := validTask
	turn0Task.Turn = 0
	err = store.saveTask(turn0Task)
	if err == nil {
		t.Fatal("expected error for turn 0, got nil")
	}

	// 7. 文件名与内容中的 ID 必须一致
	err = store.saveTask(validTask)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedPath := filepath.Join(dir, "tasks", "mismatched.json")
	mismatchedData, err := json.Marshal(validTask)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(mismatchedPath, mismatchedData, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.loadTask("mismatched")
	if err == nil {
		t.Fatal("expected error when file name does not match task ID, got nil")
	}
	_, err = store.listTasks()
	if err == nil {
		t.Fatal("expected error from listTasks when file name does not match task ID, got nil")
	}
	_ = os.Remove(mismatchedPath)

	// 8. 启动恢复遇到重复 childSessionID 必须报错拒启
	dupChildTask := validTask
	dupChildTask.ID = "task-val-2"
	err = store.saveTask(dupChildTask)
	if err != nil {
		t.Fatal(err)
	}

	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, err = newSubagentsWithStore(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, store)
	if err == nil {
		t.Fatal("expected newSubagentsWithStore to fail on duplicate childSessionID, got nil")
	}
}

func TestSubagentsOptions(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	opts, err := f.subagents.Options(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Agents) == 0 {
		t.Fatal("expected non-empty agents")
	}
	if len(opts.Models) == 0 {
		t.Fatal("expected non-empty models")
	}
}

func TestSubagentsSpawnValidation(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	// 缺少父信息
	_, err := f.subagents.Spawn(context.Background(), SpawnInput{
		Description: "test",
	})
	if !errors.Is(err, ErrParentRequired) {
		t.Fatalf("expected ErrParentRequired, got %v", err)
	}

	// 描述为空
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "   ",
	})
	if !errors.Is(err, ErrDescriptionEmpty) {
		t.Fatalf("expected ErrDescriptionEmpty, got %v", err)
	}

	// 未知 Agent
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "test",
		AgentID:         "non-existent-agent",
	})
	if err == nil {
		t.Fatal("expected error for non-existent agent, got nil")
	}

	// 未知 Model
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "test",
		Model:           "non-existent-model",
	})
	if err == nil {
		t.Fatal("expected error for non-existent model, got nil")
	}
}

func TestFindFinalAssistantEntryStrict(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	childID := "child-strict-test"
	_, err := f.sessions.Create(childID)
	if err != nil {
		t.Fatal(err)
	}

	runID := "run-strict-1"

	// 1. 没有任何 assistant 消息：不是错误，返回空位置与 nil 错误
	entryID, err := f.subagents.findFinalAssistantEntry(childID, runID)
	if err != nil || entryID != "" {
		t.Fatalf("expected empty entry and nil error when no assistant message exists, got entry=%q err=%v", entryID, err)
	}

	// 2. 有 assistant 消息但包含 tool-call：返回空位置与 nil 错误
	sess, err := f.sessions.Get(childID)
	if err != nil {
		t.Fatal(err)
	}
	toolCallMsg := session.Message{
		RunID: runID,
		Role:  session.RoleAssistant,
		Blocks: []session.Block{
			{Kind: "text", Text: "calling tool"},
			{Kind: "tool-call", Tool: &session.ToolCall{ID: "call-1", Name: "bash"}},
		},
	}
	_, err = sess.Append(toolCallMsg)
	if err != nil {
		t.Fatal(err)
	}

	entryID, err = f.subagents.findFinalAssistantEntry(childID, runID)
	if err != nil || entryID != "" {
		t.Fatalf("expected empty entry and nil error when last assistant message has tool-call, got entry=%q err=%v", entryID, err)
	}

	// 3. 消息在前面有纯文本，但最后一条依然是 tool-call 时，绝不能往前倒退捡取中间文本！
	earlierTextMsg := session.Message{
		RunID: runID,
		Role:  session.RoleAssistant,
		Blocks: []session.Block{
			{Kind: "text", Text: "intermediate text that is not final answer"},
		},
	}
	childID2 := "child-strict-test-2"
	_, err = f.sessions.Create(childID2)
	if err != nil {
		t.Fatal(err)
	}
	sess2, err := f.sessions.Get(childID2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sess2.Append(earlierTextMsg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sess2.Append(toolCallMsg)
	if err != nil {
		t.Fatal(err)
	}

	entryID, err = f.subagents.findFinalAssistantEntry(childID2, runID)
	if err != nil || entryID != "" {
		t.Fatalf("must not pick earlier intermediate text, expected empty entry, got entry=%q err=%v", entryID, err)
	}

	// 4. 纯空白或空文本：返回空位置与 nil 错误
	childID3 := "child-strict-test-3"
	_, err = f.sessions.Create(childID3)
	if err != nil {
		t.Fatal(err)
	}
	sess3, err := f.sessions.Get(childID3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sess3.Append(session.Message{
		RunID: runID,
		Role:  session.RoleAssistant,
		Blocks: []session.Block{
			{Kind: "text", Text: "   \n\t  "},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entryID, err = f.subagents.findFinalAssistantEntry(childID3, runID)
	if err != nil || entryID != "" {
		t.Fatalf("expected empty entry for whitespace-only text, got entry=%q err=%v", entryID, err)
	}

	// 5. 真正的最终无 tool-call 文本回答：返回真实 entryID
	finalMsg := session.Message{
		RunID: runID,
		Role:  session.RoleAssistant,
		Blocks: []session.Block{
			{Kind: "text", Text: "final verified answer"},
		},
	}
	_, err = sess.Append(finalMsg)
	if err != nil {
		t.Fatal(err)
	}

	entryID, err = f.subagents.findFinalAssistantEntry(childID, runID)
	if err != nil {
		t.Fatalf("expected success on valid final assistant message, got: %v", err)
	}
	if entryID == "" {
		t.Fatal("expected non-empty entryID")
	}

	// 6. 查不存在的 session 时必须返回真实错误（来自 session.Get）
	_, err = f.subagents.findFinalAssistantEntry("non-existent-session", runID)
	if err == nil {
		t.Fatal("expected error on non-existent session, got nil")
	}
}

func TestSubagentsFastCompletionAndResultBinding(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "do work",
	})
	if err != nil {
		t.Fatal(err)
	}

	f.loop.waitStarted(t)
	f.loop.release()

	waitRes, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waitRes.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %s", waitRes.Status)
	}
	if waitRes.ResultEntryID == "" {
		t.Fatal("expected non-empty ResultEntryID for completed assistant response")
	}
	if waitRes.Turn != 1 {
		t.Fatalf("expected turn 1, got %d", waitRes.Turn)
	}

	tasks, err := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != StatusCompleted {
		t.Fatalf("unexpected task in list: %+v", tasks)
	}
}

func TestSubagentsNestedDelegationRejected(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "first child",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 尝试以孩子作为父进行二级委派
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: spawnRes.ChildSessionID,
		ParentRunID:     spawnRes.RunID,
		Description:     "nested child",
	})
	if !errors.Is(err, ErrNestedDelegation) {
		t.Fatalf("expected ErrNestedDelegation, got %v", err)
	}
	f.loop.release()
}

func TestSubagentsSendIdleAndMultiTurn(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "turn 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()

	waitRes, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil || waitRes.Status != StatusCompleted {
		t.Fatalf("turn 1 wait failed: %v, status=%s", err, waitRes.Status)
	}

	// 闲时 Send 开启 Turn 2
	sendRes, err := f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "turn 2 prompt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendRes.Turn != 2 || sendRes.Steered {
		t.Fatalf("expected turn 2 not steered, got turn=%d steered=%v", sendRes.Turn, sendRes.Steered)
	}
	if sendRes.RunID == spawnRes.RunID {
		t.Fatal("turn 2 must have a new distinct RunID")
	}

	f.loop.waitStarted(t)
	f.loop.release()

	waitRes2, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil || waitRes2.Status != StatusCompleted {
		t.Fatalf("turn 2 wait failed: %v, status=%s", err, waitRes2.Status)
	}
	if waitRes2.Turn != 2 {
		t.Fatalf("expected wait result turn 2, got %d", waitRes2.Turn)
	}
}

func TestTaskPerTurnHistoryAndNotificationsAndDeepCopy(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "turn 1 description",
	})
	if err != nil {
		t.Fatal(err)
	}

	f.loop.waitStarted(t)
	f.loop.release()

	waitRes1, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waitRes1.Status != StatusCompleted {
		t.Fatalf("turn 1 status: %s", waitRes1.Status)
	}

	// 开启 Turn 2
	sendRes, err := f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "turn 2 prompt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendRes.Turn != 2 {
		t.Fatalf("expected turn 2, got %d", sendRes.Turn)
	}

	f.loop.waitStarted(t)
	f.loop.release()

	waitRes2, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waitRes2.Status != StatusCompleted {
		t.Fatalf("turn 2 status: %s", waitRes2.Status)
	}

	tasks, err := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]

	// 验证逐轮记录保留
	if len(task.Turns) != 2 {
		t.Fatalf("expected 2 turns in history, got %d", len(task.Turns))
	}
	if task.Turns[0].Turn != 1 || task.Turns[0].Status != StatusCompleted || task.Turns[0].ResultEntryID == "" {
		t.Fatalf("turn 1 record corrupted: %+v", task.Turns[0])
	}
	if task.Turns[1].Turn != 2 || task.Turns[1].Status != StatusCompleted || task.Turns[1].ResultEntryID == "" {
		t.Fatalf("turn 2 record corrupted: %+v", task.Turns[1])
	}
	if task.Turns[0].RunID == task.Turns[1].RunID {
		t.Fatal("turn 1 and turn 2 must have different RunIDs")
	}

	// 验证通知集合保留
	if len(task.Notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(task.Notifications))
	}
	if task.Notifications[0].Turn != 1 || task.Notifications[1].Turn != 2 {
		t.Fatalf("unexpected notifications: %+v", task.Notifications)
	}

	// 验证深拷贝：修改返回的 task 不影响内部状态
	tasks[0].Turns[0].Status = StatusFailed
	tasks[0].Notifications[0].Status = StatusFailed

	tasksAfter, err := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if tasksAfter[0].Turns[0].Status != StatusCompleted {
		t.Fatal("task.Turns was modified by external mutation (shallow copy detected)")
	}
	if tasksAfter[0].Notifications[0].Status != StatusCompleted {
		t.Fatal("task.Notifications was modified by external mutation (shallow copy detected)")
	}
}

func TestSubagentsSendRunningSteers(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "running child",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 忙碌时 Send：必须通过 Steer
	sendRes, err := f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "steer child"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sendRes.Steered || sendRes.Turn != 1 {
		t.Fatalf("expected Steered=true, turn=1, got %+v", sendRes)
	}

	f.loop.release()
	waitRes, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil || waitRes.Status != StatusCompleted {
		t.Fatalf("wait after steer failed: %v", err)
	}
}

func TestSendBarrierPreventsOldTurnDropped(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "turn 1 barrier test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 利用 store 自身的锁构造确定性 barrier：
	// 在 handle 完成退出后、在 onRunFinished 执行 saveTask 前阻塞它
	coord := f.subagents.coords[spawnRes.TaskID]
	coord.mu.Lock()
	handle := coord.activeHandle
	coord.mu.Unlock()
	f.subagents.store.mu.Lock()

	// 释放 runner loop，使其运行完成，handle.Done() 即将就绪
	f.loop.release()

	// 确认 Runner 已完成；回调无论先后，都被 store 锁挡在最终落盘之前。
	awaitSignal(t, handle.Done())

	// 在后台发起 Send（模拟并发请求新轮次）
	sendDone := make(chan struct{})
	var sendRes SendResult
	var sendErr error
	go func() {
		defer close(sendDone)
		sendRes, sendErr = f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
			Blocks: []session.Block{{Kind: "text", Text: "turn 2 prompt"}},
		})
	}()

	// 此时 Send 必须正在等待 finalizingCh，绝不能提前返回并破坏 Turn 1
	select {
	case <-sendDone:
		f.subagents.store.mu.Unlock()
		t.Fatal("Send returned prematurely before old turn was finalized!")
	case <-time.After(100 * time.Millisecond):
		// 符合预期，Send 正在 barrier 前等待旧轮次收尾
	}

	// 释放 store 锁，允许 onRunFinished 完成落盘
	f.subagents.store.mu.Unlock()

	// 等待 Send 完成
	select {
	case <-sendDone:
		if sendErr != nil {
			t.Fatalf("Send failed: %v", sendErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Send did not finish within timeout")
	}

	if sendRes.Turn != 2 {
		t.Fatalf("expected turn 2, got %d", sendRes.Turn)
	}

	// 此时验证：Turn 1 已经完整落盘，保留在 Turns 中且状态为 Completed，并且旧 waitCh 已被关闭
	tasks, err := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if len(task.Turns) < 2 {
		t.Fatalf("expected at least 2 turns recorded, got %d", len(task.Turns))
	}
	if task.Turns[0].Turn != 1 || task.Turns[0].Status != StatusCompleted || task.Turns[0].ResultEntryID == "" {
		t.Fatalf("turn 1 was dropped or corrupted by premature Send: %+v", task.Turns[0])
	}
	if len(task.Notifications) < 1 {
		t.Fatal("expected turn 1 notification to be preserved")
	}

	// 释放 Turn 2 loop 完成整个生命周期
	f.loop.waitStarted(t)
	f.loop.release()

	waitRes2, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waitRes2.Status != StatusCompleted {
		t.Fatalf("turn 2 status: %s", waitRes2.Status)
	}
}

func TestWaitReturnsCompletedTurnDespiteImmediateSend(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "wait turn pinning test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 启动一个正在等待 Turn 1 完成的 Wait
	waitDone := make(chan WaitResult, 1)
	waitErrCh := make(chan error, 1)
	waitCtx := &observedWaitContext{Context: context.Background(), entered: make(chan struct{})}
	go func() {
		res, err := f.subagents.Wait(waitCtx, parentSessionID, spawnRes.TaskID, 3*time.Second)
		if err != nil {
			waitErrCh <- err
			return
		}
		waitDone <- res
	}()
	awaitSignal(t, waitCtx.entered)

	// 释放 Turn 1 让其完成
	f.loop.release()

	// 在 Turn 1 刚完成后立即并发发起 Send 开启 Turn 2
	var sendRes SendResult
	for i := 0; i < 50; i++ {
		sendRes, err = f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
			Blocks: []session.Block{{Kind: "text", Text: "turn 2 prompt"}},
		})
		if err == nil && !sendRes.Steered {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Send turn 2 failed: %v", err)
	}
	if sendRes.Turn != 2 {
		t.Fatalf("expected turn 2, got %d", sendRes.Turn)
	}

	// 验证先前发起的 Wait 依然精准返回 Turn 1 的 Completed 结果，而不是 Turn 2 的 Running 状态
	select {
	case err := <-waitErrCh:
		t.Fatalf("Wait failed: %v", err)
	case res := <-waitDone:
		if res.Turn != 1 {
			t.Fatalf("Wait was waiting for turn 1, but returned turn %d", res.Turn)
		}
		if res.Status != StatusCompleted {
			t.Fatalf("Wait expected status completed, got %s", res.Status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait timed out")
	}

	// 收尾 Turn 2
	f.loop.waitStarted(t)
	f.loop.release()
}

func TestListConcurrentWithSendAndFinish(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "race list test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 并发执行 List 与 Send / 完成，检测锁序与数据竞态
	stopList := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopList:
				return
			default:
				_, _ = f.subagents.List(parentSessionID, spawnRes.TaskID)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	f.loop.release()

	_, err = f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "turn 2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()

	_, err = f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	close(stopList)
	wg.Wait()
}

func TestSubagentsStopAndStopFamily(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "child to stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	err = f.subagents.Stop(context.Background(), parentSessionID, spawnRes.TaskID)
	if err != nil {
		t.Fatal(err)
	}

	waitRes, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if waitRes.Status != StatusCancelled {
		t.Fatalf("expected cancelled status, got %s", waitRes.Status)
	}
	if waitRes.ResultEntryID != "" {
		t.Fatal("cancelled run must not have ResultEntryID")
	}

	// 取消后依然允许明确 Send 开启新轮次
	sendRes, err := f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "new turn after cancel"}},
	})
	if err != nil {
		t.Fatalf("send after cancel failed: %v", err)
	}
	if sendRes.Turn != 2 {
		t.Fatalf("expected turn 2, got %d", sendRes.Turn)
	}
	f.loop.waitStarted(t)
	f.loop.release()

	// StopFamily 测试
	err = f.subagents.StopFamily(context.Background(), parentSessionID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCloseBlockedSpawnAndConcurrency(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	// 利用 barrierSettingsStore 在 Put 时卡住 Spawn
	inPut := make(chan struct{})
	unblockPut := make(chan struct{})
	barrierStore := &barrierSettingsStore{
		SessionSettingsStore: f.settings,
		beforePut: func(sessionID string) {
			close(inPut)
			<-unblockPut
		},
	}
	f.subagents.settings = barrierStore

	spawnDone := make(chan struct{})
	var spawnErr error
	go func() {
		defer close(spawnDone)
		_, spawnErr = f.subagents.Spawn(context.Background(), SpawnInput{
			ParentSessionID: parentSessionID,
			ParentRunID:     parentRunID,
			Description:     "blocked spawn",
		})
	}()

	// 等待 Spawn 运行到 Put 阻塞点
	select {
	case <-inPut:
	case <-time.After(3 * time.Second):
		t.Fatal("Spawn did not enter Put barrier in time")
	}

	// 并发执行多个 Close()
	const numClosers = 5
	closeErrs := make(chan error, numClosers)
	var wg sync.WaitGroup
	wg.Add(numClosers)
	for i := 0; i < numClosers; i++ {
		go func() {
			defer wg.Done()
			closeErrs <- f.subagents.Close()
		}()
	}

	// 服务已经开始关闭，但仍需等待被阻塞的 Spawn。
	awaitSignal(t, f.subagents.ctx.Done())

	// 放开 Put
	close(unblockPut)
	<-spawnDone
	if !errors.Is(spawnErr, ErrClosed) {
		t.Fatalf("expected ErrClosed for blocked spawn, got %v", spawnErr)
	}

	wg.Wait()
	close(closeErrs)

	// 验证所有 Close() 均拿到了相同的返回结果
	var firstErr error
	for err := range closeErrs {
		if firstErr == nil {
			firstErr = err
		} else if err != firstErr {
			t.Fatalf("inconsistent Close results: %v vs %v", err, firstErr)
		}
	}

	// 验证后续请求被拒绝
	_, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "after close",
	})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestSpawnInitialPersistFailureConsistency(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	tasksDir := filepath.Join(f.dataDir, "subagents", "tasks")
	err := os.Chmod(tasksDir, 0o555)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(tasksDir, 0o755) }()

	// 初始落盘失败
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "initial save fail test",
	})
	if err == nil {
		t.Fatal("expected Spawn to fail when directory is not writable")
	}
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("expected ErrPersistFailed, got %v", err)
	}

	// 恢复目录可写
	_ = os.Chmod(tasksDir, 0o755)

	// 验证事实一致性：子会话索引保留在内存中，ChatService.IsChildSession 依然为 true，且没有建立 Session
	taskList, err := f.subagents.List(parentSessionID, "")
	if err != nil {
		// List 应该能列出该失败记录，或者报 persistErr
	}
	if len(taskList) != 1 {
		t.Fatalf("expected 1 recorded task in memory, got %d", len(taskList))
	}
	failedTask := taskList[0]
	if failedTask.Status != StatusFailed {
		t.Fatalf("expected task status failed, got %s", failedTask.Status)
	}
	if !f.subagents.IsChildSession(failedTask.ChildSessionID) {
		t.Fatal("child session must remain registered in IsChildSession to isolate from chat")
	}

	// 验证 sessions 存储中并未创建该子 session
	_, err = f.sessions.Get(failedTask.ChildSessionID)
	if err == nil {
		t.Fatal("child session should not have been created in session.Store")
	}
}

func TestOnRunFinishedSaveFailure(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "terminal save failure test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// 在文件系统上制造真实障碍：将 taskID.json.tmp 建成目录，阻断 OpenFile 写临时文件
	tasksDir := filepath.Join(f.dataDir, "subagents", "tasks")
	tmpDirPath := filepath.Join(tasksDir, spawnRes.TaskID+".json.tmp")
	err = os.Mkdir(tmpDirPath, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpDirPath) }()

	// 释放 runner，使任务执行完成，触发 onRunFinished 终态落盘
	f.loop.release()

	// Wait 必须感知落盘失败并唤醒退出，绝不能死等
	waitRes, waitErr := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if waitErr == nil {
		t.Fatalf("expected Wait to fail with ErrPersistFailed, got res=%+v", waitRes)
	}
	if !errors.Is(waitErr, ErrPersistFailed) {
		t.Fatalf("expected ErrPersistFailed, got %v", waitErr)
	}

	// List 也必须如实汇报 persistErr
	_, listErr := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if listErr == nil {
		t.Fatal("expected List to report persistErr")
	}

	// Close 也必须上报 persistErr
	closeErr := f.subagents.Close()
	if closeErr == nil {
		t.Fatal("expected Close to report persistErr")
	}
}

func TestSendPendingSaveFailure(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "send save failure test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()

	waitRes, err := f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, 3*time.Second)
	if err != nil || waitRes.Status != StatusCompleted {
		t.Fatalf("turn 1 wait failed: %v", err)
	}

	// 制造真实文件系统障碍阻断 Send 的 pending 保存
	tasksDir := filepath.Join(f.dataDir, "subagents", "tasks")
	tmpDirPath := filepath.Join(tasksDir, spawnRes.TaskID+".json.tmp")
	err = os.Mkdir(tmpDirPath, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpDirPath) }()

	// Send 开启 Turn 2 应在 saveTask pending 时失败
	_, err = f.subagents.Send(context.Background(), parentSessionID, spawnRes.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "turn 2 prompt"}},
	})
	if err == nil {
		t.Fatal("expected Send to fail when pending turn save fails")
	}
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("expected ErrPersistFailed, got %v", err)
	}
	_, err = f.subagents.Wait(context.Background(), parentSessionID, spawnRes.TaskID, time.Second)
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("pending failure not reported by Wait: %v", err)
	}
	tasks, err := f.subagents.List(parentSessionID, spawnRes.TaskID)
	if err == nil || len(tasks) != 1 || tasks[0].Turns[1].Status != StatusFailed {
		t.Fatalf("pending failure not retained: %+v, %v", tasks, err)
	}
	err = f.subagents.Close()
	if err == nil {
		t.Fatal("pending failure not reported by Close")
	}
}

func TestSubagentsCloseStopsAndExitsCleanly(t *testing.T) {
	f := newSubagentsFixture(t)

	workspace := t.TempDir()
	parentSessionID, parentRunID := createParentRun(t, f, workspace)
	defer f.loop.releaseParent()

	spawnRes, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "child for close test",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)

	// Close 必须取消孩子并等待自身后台 goroutine 退出
	done := make(chan error, 1)
	go func() {
		done <- f.subagents.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close did not finish in time (goroutine leak or deadlock)")
	}

	// 关闭后新请求被拒绝
	_, err = f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parentSessionID,
		ParentRunID:     parentRunID,
		Description:     "after close",
	})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
	_ = f.host.Close()
	_ = spawnRes
}
