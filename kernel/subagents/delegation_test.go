package subagents

import (
	"context"
	"errors"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

func TestSendRejectsBlankBeforeStartingTurn(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.close()
	parent, run := createParentRun(t, f, t.TempDir())
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		TaskName:        "test",
		ParentSessionID: parent, ParentRunID: run, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	childSession, err := f.sessions.Get(child.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	first := childSession.Entries()[0].Message
	if first.UserAuthored || first.SourceSessionID != parent || first.SourceRunID != run {
		t.Fatal("delegation must not become user authorization")
	}
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.subagents.Send(context.Background(), parent, run, child.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "   "}},
	})
	if err == nil {
		t.Fatal("blank send accepted")
	}
	tasks, err := f.subagents.List(parent, child.TaskID)
	if err != nil || tasks[0].Turn != 1 {
		t.Fatalf("blank send changed task: %+v, %v", tasks, err)
	}
}

func TestInheritedIncompatibleEffortIsNotSilentlyReplaced(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.close()
	_, err := f.sessions.Create("parent-session")
	if err != nil {
		t.Fatal(err)
	}
	// 自定义 Loop 不请求 LLM，模拟父快照中存在目标模型不支持的档位。
	err = f.settings.Put("parent-session", settings.SessionSettings{
		AgentID: agents.DefaultID, Model: "deepseek/deepseek-flash",
		ReasoningEffort: "unsupported", Workspace: t.TempDir(),
		PermissionMode: "read_only",
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := f.runner.Start(context.Background(), "parent-session", session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "parent"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitParentStarted(t)
	input := SpawnInput{
		TaskName: "test", ParentSessionID: "parent-session", ParentRunID: handle.RunID(),
		Description: "child", Model: "deepseek/deepseek-v4-pro"}
	_, err = f.subagents.Spawn(context.Background(), input)
	if err == nil {
		t.Fatal("incompatible inherited effort was silently replaced")
	}
	tasks, err := f.subagents.List("parent-session", "")
	if err != nil || len(tasks) != 0 {
		t.Fatalf("invalid settings created a child: %+v, %v", tasks, err)
	}
	input.ReasoningEffort = "high"
	// 父设置之后变化也不能改变本轮的继承来源。
	changed, err := f.settings.For("parent-session")
	if err != nil {
		t.Fatal(err)
	}
	changed.PermissionMode = "full_access"
	err = f.settings.Put("parent-session", changed)
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.subagents.Spawn(context.Background(), input)
	if err != nil {
		t.Fatalf("explicit compatible effort rejected: %v", err)
	}
	childSettings, err := f.settings.For(child.ChildSessionID)
	if err != nil || childSettings.PermissionMode != "read_only" {
		t.Fatalf("parent run mode not inherited: %+v, %v", childSettings, err)
	}
}

func TestUserCanContinueChildAfterParentRunEnds(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.close()
	parentSessionID, parentRunID := createParentRun(t, f, t.TempDir())
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		TaskName: "继续实现", ParentSessionID: parentSessionID,
		ParentRunID: parentRunID, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parentSessionID, WaitInput{
		TaskIDs: []string{child.TaskID}, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	f.loop.releaseParent()
	deadline := time.Now().Add(time.Second)
	for {
		if _, active := f.runner.State(parentSessionID); !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parent run did not finish")
		}
		time.Sleep(time.Millisecond)
	}

	result, err := f.subagents.SendFromUser(context.Background(), parentSessionID, child.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "continue after parent"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Turn != 2 || result.Steered {
		t.Fatalf("expected a second child turn, got %+v", result)
	}
	f.loop.waitStarted(t)
	childSession, err := f.sessions.Get(child.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range childSession.Entries() {
		if entry.Message.RunID == result.RunID && entry.Message.Role == session.RoleUser {
			found = entry.Message.UserAuthored && entry.Message.SourceSessionID == ""
		}
	}
	if !found {
		t.Fatal("direct user input lost its trusted origin")
	}
	f.loop.release()
}

func TestTaskSettingsOnlyChangeWhenChildIsIdle(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.close()
	parentSessionID, parentRunID := createParentRun(t, f, t.TempDir())
	defer f.loop.releaseParent()
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		TaskName: "设置测试", ParentSessionID: parentSessionID,
		ParentRunID: parentRunID, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	_, err = f.subagents.UpdateSettings(context.Background(), parentSessionID, child.TaskID, TaskSettingsInput{
		Model: "deepseek/deepseek-v4-pro", ReasoningEffort: "high",
	})
	if !errors.Is(err, ErrTaskActive) {
		t.Fatalf("active child settings update returned %v", err)
	}
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parentSessionID, WaitInput{
		TaskIDs: []string{child.TaskID}, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.subagents.UpdateSettings(context.Background(), parentSessionID, child.TaskID, TaskSettingsInput{
		Model: "deepseek/deepseek-v4-pro", ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.AgentID != agents.DefaultID || next.Model != "deepseek/deepseek-v4-pro" || next.Workspace == "" {
		t.Fatalf("fixed settings changed or model was not saved: %+v", next)
	}
}
