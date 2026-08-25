package web

import (
	"encoding/json"
	"net/url"
	"sort"

	"github.com/a-h/templ"

	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
)

// PageData 是聊天主页面渲染所需的只读材料。
type PageData struct {
	Projects     []projects.Project
	ProjectTrees []ProjectTree
	Project      projects.Project
	Sessions     []session.Header
	Presets      []presets.Preset
	Providers    []llm.ProviderInfo
	Header       session.Header
	Chat         []ChatItem

	SelectedPreset presets.Preset
	SelectedModel  llm.Selection
	HasProject     bool
	HasSession     bool
	Busy           bool
	HasPreset      bool
	Draft          bool
	Error          string
}

// ProjectTree 是侧栏里一个项目及其直属会话。
type ProjectTree struct {
	Project  projects.Project
	Sessions []session.Header
}

// PresetPageData 是 Agent 模式管理页面的材料。
type PresetPageData struct {
	Presets []presets.Preset
	Tools   []string
	Error   string
}

// ChatItem 是从账本事件投影出来的一条可见内容。
type ChatItem struct {
	Kind       string
	Text       string
	HTML       string
	ToolName   string
	ToolStatus string
}

func projectEvents(events []session.Event) []ChatItem {
	replaced := make(map[int]bool)
	claimed := make(map[string]bool)
	for _, event := range events {
		for _, seq := range event.Replaces {
			replaced[seq] = true
		}
		if event.Kind == session.KindClaim {
			var data session.ClaimData
			if json.Unmarshal(event.Data, &data) == nil {
				claimed[data.ID] = true
			}
		}
	}

	toolNames := make(map[string]string)
	toolItems := make(map[string]int)
	items := make([]ChatItem, 0)
	chunk := ""
	flushChunk := func() {
		if chunk == "" {
			return
		}
		items = append(items, ChatItem{Kind: "assistant streaming", Text: chunk, HTML: renderMarkdown(chunk)})
		chunk = ""
	}
	for _, event := range events {
		if replaced[event.Seq] {
			continue
		}
		switch event.Kind {
		case session.KindUserMessage:
			flushChunk()
			var data session.UserMessageData
			if json.Unmarshal(event.Data, &data) == nil {
				items = append(items, ChatItem{Kind: "user", Text: data.Text})
			}
		case session.KindChunk:
			var data session.ChunkData
			if json.Unmarshal(event.Data, &data) == nil {
				chunk += data.Delta
			}
		case session.KindAssistantFinal:
			flushChunk()
			var data session.AssistantFinalData
			if json.Unmarshal(event.Data, &data) == nil {
				kind := "assistant"
				if data.Interrupted {
					kind = "assistant interrupted"
				}
				items = append(items, ChatItem{Kind: kind, Text: data.Text, HTML: renderMarkdown(data.Text)})
			}
		case session.KindToolCall:
			flushChunk()
			var data session.ToolCallData
			if json.Unmarshal(event.Data, &data) == nil {
				toolNames[data.ID] = data.Name
				toolItems[data.ID] = len(items)
				items = append(items, ChatItem{Kind: "tool", ToolName: data.Name, ToolStatus: "准备调用"})
			}
		case session.KindToolStart:
			var data session.ToolStartData
			if json.Unmarshal(event.Data, &data) == nil {
				index, exists := toolItems[data.CallID]
				if exists {
					items[index].ToolStatus = "运行中"
				}
			}
		case session.KindToolResult:
			var data session.ToolResultData
			if json.Unmarshal(event.Data, &data) == nil {
				index, exists := toolItems[data.CallID]
				if exists {
					items[index].ToolStatus = toolResultLabel(data.Status)
					items[index].Text = data.Output
				} else {
					items = append(items, ChatItem{Kind: "tool", ToolName: toolNames[data.CallID], ToolStatus: toolResultLabel(data.Status), Text: data.Output})
				}
			}
		case session.KindDeliver:
			var data session.DeliverData
			if json.Unmarshal(event.Data, &data) == nil && !claimed[data.ID] {
				items = append(items, ChatItem{Kind: "queued", Text: data.Text})
			}
		}
	}
	flushChunk()
	return items
}

func toolResultLabel(status string) string {
	switch status {
	case session.ResultSuccess:
		return "已完成"
	case session.ResultFailed:
		return "失败"
	case session.ResultSkipped:
		return "已跳过"
	default:
		return "未知"
	}
}

func activePresets(listed []presets.Preset) []presets.Preset {
	active := make([]presets.Preset, 0, len(listed))
	for _, preset := range listed {
		if !preset.Archived {
			active = append(active, preset)
		}
	}
	sort.Slice(active, func(i int, j int) bool { return active[i].ID < active[j].ID })
	return active
}

func projectClass(id string, selected string) string {
	if id == selected {
		return "project-link selected"
	}
	return "project-link"
}

func sessionClass(id string, selected string) string {
	if id == selected {
		return "session-link selected"
	}
	return "session-link"
}

func newChatURL(data PageData) templ.SafeURL {
	if data.HasProject {
		return templ.URL("/?project=" + data.Project.ID + "&draft=1")
	}
	return templ.URL("/?draft=1")
}

func presetAction(id string) string {
	if id == "" {
		return "/presets"
	}
	return "/presets/update"
}

func presetEditURL(id string) string {
	return "/presets/edit?id=" + url.QueryEscape(id)
}

func presetHasTool(preset presets.Preset, name string) bool {
	for _, allowed := range preset.Tools {
		if allowed == name {
			return true
		}
	}
	return false
}

func composerFooterClass(hasSession bool) string {
	if hasSession {
		return "composer-footer session-composer-footer"
	}
	return "composer-footer"
}

func modelBaseKey(provider string, modelID string) string {
	return provider + "\x1f" + modelID
}

func currentModelBaseKey(selection llm.Selection) string {
	return modelBaseKey(selection.Provider, selection.Model)
}

type thinkingChoice struct {
	Value string
	Label string
}

func selectedModelInfo(providers []llm.ProviderInfo, selection llm.Selection) (llm.ModelInfo, bool) {
	for _, provider := range providers {
		if provider.Name != selection.Provider {
			continue
		}
		for _, model := range provider.Models {
			if model.ID != selection.Model {
				continue
			}
			return model, true
		}
	}
	return llm.ModelInfo{}, false
}

func providerDisplayName(provider llm.ProviderInfo) string {
	if provider.DisplayName != "" {
		return provider.DisplayName
	}
	return provider.Name
}

func modelDisplayName(providers []llm.ProviderInfo, selection llm.Selection) string {
	model, found := selectedModelInfo(providers, selection)
	if found && model.Name != "" {
		return model.Name
	}
	if selection.Model != "" {
		return selection.Model
	}
	return "选择模型"
}

func thinkingLabel(value string) string {
	switch value {
	case "":
		return "Default"
	case "off":
		return "Off"
	case "low":
		return "Low"
	case "high":
		return "High"
	case "max":
		return "Max"
	default:
		return value
	}
}

func thinkingChoices(providers []llm.ProviderInfo, selection llm.Selection) []thinkingChoice {
	model, found := selectedModelInfo(providers, selection)
	if !found {
		return nil
	}
	choices := make([]thinkingChoice, 0, len(model.ThinkingLevels)+1)
	if model.SupportsProviderDefault {
		choices = append(choices, thinkingChoice{Label: "Default"})
	}
	for _, level := range model.ThinkingLevels {
		choices = append(choices, thinkingChoice{Value: level, Label: thinkingLabel(level)})
	}
	return choices
}

func selectedModelOption(selection llm.Selection, providerName string, modelID string) bool {
	return selection.Provider == providerName && selection.Model == modelID
}

func selectedThinkingOption(selection llm.Selection, value string) bool {
	return selection.Thinking == value
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
