package web

import (
	"net/url"
	"sort"

	"github.com/a-h/templ"

	"harness/llm"
	"harness/presets"
)

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
