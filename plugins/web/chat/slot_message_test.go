package chat

import (
	"context"
	"testing"

	"harness/kernel/session"
)

func TestMessageActionRegistryRegistersAndSortsDefinitions(t *testing.T) {
	registry := newMessageActionRegistry()
	err := registry.RegisterMessageAction(testMessageAction{definition: MessageActionDefinition{
		ID: "second", Name: "第二", Icon: "2", Order: 20, Targets: []MessageCardType{MessageCardUser},
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterMessageAction(testMessageAction{definition: MessageActionDefinition{
		ID: "first", Name: "第一", Icon: "1", Order: 10, Targets: []MessageCardType{MessageCardAssistant},
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterMessageAction(testMessageAction{definition: MessageActionDefinition{
		ID: "alpha", Name: "甲", Icon: "A", Order: 10, Targets: []MessageCardType{MessageCardUser},
	}})
	if err != nil {
		t.Fatal(err)
	}
	actions := registry.MessageActions()
	if len(actions) != 3 || actions[0].ID != "alpha" || actions[1].ID != "first" || actions[2].ID != "second" {
		t.Fatalf("actions = %#v", actions)
	}
	if _, ok := registry.MessageAction("first"); !ok {
		t.Fatal("first action is missing")
	}
}

func TestMessageActionRegistryRejectsInvalidAndDuplicateActions(t *testing.T) {
	registry := newMessageActionRegistry()
	if err := registry.RegisterMessageAction(nil); err == nil {
		t.Fatal("RegisterMessageAction(nil) succeeded")
	}
	invalid := testMessageAction{definition: MessageActionDefinition{ID: "invalid"}}
	if err := registry.RegisterMessageAction(invalid); err == nil {
		t.Fatal("RegisterMessageAction(invalid) succeeded")
	}
	invalidTarget := testMessageAction{definition: MessageActionDefinition{
		ID: "invalid-target", Name: "错误", Icon: "X", Targets: []MessageCardType{"other"},
	}}
	if err := registry.RegisterMessageAction(invalidTarget); err == nil {
		t.Fatal("RegisterMessageAction(invalidTarget) succeeded")
	}
	valid := testMessageAction{definition: MessageActionDefinition{
		ID: "copy", Name: "复制", Icon: "C", Targets: []MessageCardType{MessageCardUser},
	}}
	err := registry.RegisterMessageAction(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterMessageAction(valid); err == nil {
		t.Fatal("RegisterMessageAction accepted duplicate")
	}
}

func TestCopyMessageActionKeepsAssistantSegmentInsideSteerBoundary(t *testing.T) {
	entries := []session.Entry{
		entry("question", "run-1", session.RoleUser, "开始"),
		entry("draft", "run-1", session.RoleAssistant, "先查一下", "推理不复制"),
		entry("tool", "run-1", session.RoleTool, ""),
		entry("first", "run-1", session.RoleAssistant, "第一段"),
		entry("steer", "run-1", session.RoleUser, "继续"),
		entry("second", "run-1", session.RoleAssistant, "第二段"),
	}
	action := copyMessageAction{}
	result, err := action.Execute(context.Background(), MessageActionContext{
		Entries: entries,
		Target:  MessageActionTarget{CardType: MessageCardAssistant, RunID: "run-1", BoundaryEntryID: "question"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "第一段" {
		t.Fatalf("first assistant copy = %q", result.Text)
	}
	result, err = action.Execute(context.Background(), MessageActionContext{
		Entries: entries,
		Target:  MessageActionTarget{CardType: MessageCardAssistant, RunID: "run-1", BoundaryEntryID: "steer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "第二段" {
		t.Fatalf("second assistant copy = %q", result.Text)
	}
	result, err = action.Execute(context.Background(), MessageActionContext{
		Entries: entries,
		Target:  MessageActionTarget{CardType: MessageCardUser, EntryID: "question"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "开始" {
		t.Fatalf("user copy = %q", result.Text)
	}
}

type testMessageAction struct {
	definition MessageActionDefinition
}

func (a testMessageAction) Definition() MessageActionDefinition {
	return a.definition
}

func (testMessageAction) Execute(context.Context, MessageActionContext) (MessageActionResult, error) {
	return MessageActionResult{}, nil
}

func entry(id, runID string, role session.Role, texts ...string) session.Entry {
	blocks := make([]session.Block, 0, len(texts))
	for index, text := range texts {
		kind := "text"
		if index > 0 {
			kind = "reasoning"
		}
		blocks = append(blocks, session.Block{Kind: kind, Text: text})
	}
	return session.Entry{ID: id, Message: session.Message{RunID: runID, Role: role, Blocks: blocks}}
}
