package subagents

import (
	"context"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

func TestSendRejectsBlankBeforeStartingTurn(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	parent, run := createParentRun(t, f, t.TempDir())
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parent, ParentRunID: run, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.subagents.Send(context.Background(), parent, child.TaskID, session.UserMessage{
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
	defer f.host.Close()
	_, err := f.sessions.Create("parent-session")
	if err != nil {
		t.Fatal(err)
	}
	// 自定义 Loop 不请求 LLM，模拟父快照中存在目标模型不支持的档位。
	err = f.settings.Put("parent-session", settings.SessionSettings{
		AgentID: agents.DefaultID, Model: "deepseek/deepseek-v4-flash",
		ReasoningEffort: "unsupported", Workspace: t.TempDir(),
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
	input := SpawnInput{ParentSessionID: "parent-session", ParentRunID: handle.RunID(),
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
	_, err = f.subagents.Spawn(context.Background(), input)
	if err != nil {
		t.Fatalf("explicit compatible effort rejected: %v", err)
	}
}
