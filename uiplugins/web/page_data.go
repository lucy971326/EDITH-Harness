package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"harness/llm"
	"harness/presets"
	"harness/session"
)

func (s *service) pageData(projectID string, sessionID string, draft string, message string) (PageData, error) {
	listedProjects, err := s.projects.List()
	if err != nil {
		return PageData{}, err
	}
	data := PageData{Projects: listedProjects, Error: message}
	data.Providers = s.llm.Providers()
	for _, project := range listedProjects {
		if project.Archived {
			continue
		}
		headers, listErr := s.projects.ListSessions(project.ID)
		if listErr != nil {
			return PageData{}, listErr
		}
		sort.Slice(headers, func(i int, j int) bool {
			return headers[i].CreatedAt.After(headers[j].CreatedAt)
		})
		data.ProjectTrees = append(data.ProjectTrees, ProjectTree{Project: project, Sessions: headers})
	}
	if projectID == "" {
		for _, project := range listedProjects {
			if !project.Archived {
				projectID = project.ID
				break
			}
		}
	}
	if projectID == "" {
		return data, nil
	}
	project, err := s.projects.Get(projectID)
	if err != nil {
		return PageData{}, err
	}
	data.Project = project
	data.HasProject = true
	data.Sessions, err = s.projects.ListSessions(projectID)
	if err != nil {
		return PageData{}, err
	}
	sort.Slice(data.Sessions, func(i int, j int) bool {
		return data.Sessions[i].CreatedAt.After(data.Sessions[j].CreatedAt)
	})
	data.Presets, err = s.presets.List()
	if err != nil {
		return PageData{}, err
	}
	data.SelectedPreset = pickPreset(data.Presets, project.LastPresetID)
	data.SelectedModel = project.LastModel
	if data.SelectedModel.Provider == "" {
		data.SelectedModel, err = s.llm.DefaultSelection()
		if err != nil {
			return PageData{}, err
		}
	}
	data.HasPreset = data.SelectedPreset.ID != ""

	if sessionID == "" {
		data.Draft = draft == "1"
		return data, nil
	}
	header, events, err := s.books.Read(sessionID)
	if err != nil {
		return PageData{}, err
	}
	if header.ProjectID != projectID {
		return PageData{}, fmt.Errorf("会话 %s 不属于项目 %s", sessionID, projectID)
	}
	data.Header = header
	data.HasSession = true
	data.Chat = projectEvents(events)
	conversation, stateErr := s.agents.GetSession(sessionID)
	if stateErr == nil {
		state := conversation.State()
		data.Busy = state == "busy"
	}
	if selected, found := latestModelSelection(events); found {
		data.SelectedModel = selected
	}
	locked, lockErr := s.presets.GetRevision(header.PresetID, header.PresetRevision)
	if lockErr == nil {
		data.SelectedPreset = locked
		data.HasPreset = true
	}
	return data, nil
}

func (s *service) presetPageData(message string) (PresetPageData, error) {
	listed, err := s.presets.List()
	if err != nil {
		return PresetPageData{}, err
	}
	return PresetPageData{
		Presets: listed,
		Tools:   s.tools.Names(),
		Error:   message,
	}, nil
}

func pickPreset(listed []presets.Preset, preferred string) presets.Preset {
	for _, preset := range listed {
		if preset.ID == preferred && !preset.Archived {
			return preset
		}
	}
	for _, preset := range listed {
		if !preset.Archived {
			return preset
		}
	}
	return presets.Preset{}
}

func presetFromForm(request *http.Request) (presets.Preset, error) {
	err := request.ParseForm()
	if err != nil {
		return presets.Preset{}, err
	}
	preset := presets.Preset{
		ID:           strings.TrimSpace(request.Form.Get("id")),
		SystemPrompt: strings.TrimSpace(request.Form.Get("system_prompt")),
		Tools:        request.Form["tools"],
	}
	return preset, nil
}

func (s *service) parseSelectionFromForm(form url.Values, fallback llm.Selection) (llm.Selection, error) {
	modelKey := form.Get("model_id")
	thinking := strings.TrimSpace(form.Get("thinking"))
	_, thinkingProvided := form["thinking"]

	var provider, model string
	if modelKey != "" {
		parts := strings.Split(modelKey, "\x1f")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			provider = parts[0]
			model = parts[1]
		}
	} else if fallback.Provider != "" && fallback.Model != "" {
		provider = fallback.Provider
		model = fallback.Model
	}

	if provider != "" && model != "" {
		sameModel := provider == fallback.Provider && model == fallback.Model
		if !thinkingProvided && sameModel {
			thinking = fallback.Thinking
			thinkingProvided = true
		}
		if thinkingProvided && thinking == "" && !sameModel {
			thinkingProvided = false
		}
		resolvedThinking, err := s.resolveThinking(provider, model, thinking, thinkingProvided)
		if err != nil {
			return llm.Selection{}, err
		}
		selection := llm.Selection{
			Provider: provider,
			Model:    model,
			Thinking: resolvedThinking,
		}
		err = s.llm.Validate(selection)
		if err != nil {
			return llm.Selection{}, err
		}
		return selection, nil
	}

	if raw := form.Get("model_selection"); raw != "" {
		return s.parseSelection(raw)
	}
	if fallback.Provider != "" && fallback.Model != "" {
		return fallback, nil
	}
	return s.llm.DefaultSelection()
}

func (s *service) resolveThinking(providerName string, modelID string, preferredThinking string, thinkingProvided bool) (string, error) {
	for _, provider := range s.llm.Providers() {
		if provider.Name != providerName {
			continue
		}
		for _, model := range provider.Models {
			if model.ID != modelID {
				continue
			}
			if preferredThinking == "" {
				if model.SupportsProviderDefault {
					return "", nil
				}
				if thinkingProvided {
					return "", fmt.Errorf("模型 %s 不支持服务商默认思考档位", modelID)
				}
				if len(model.ThinkingLevels) == 0 {
					return "", fmt.Errorf("模型 %s 没有可用的思考档位", modelID)
				}
				return model.ThinkingLevels[0], nil
			}
			for _, level := range model.ThinkingLevels {
				if level == preferredThinking {
					return level, nil
				}
			}
			if len(model.ThinkingLevels) == 0 {
				return "", fmt.Errorf("模型 %s 没有可用的思考档位", modelID)
			}
			return model.ThinkingLevels[0], nil
		}
	}
	return "", fmt.Errorf("服务商 %s 没有模型 %s", providerName, modelID)
}

func (s *service) currentOrProjectModel(projectID string, sessionID string) llm.Selection {
	if sessionID != "" {
		_, events, err := s.books.Read(sessionID)
		if err == nil {
			selected, found := latestModelSelection(events)
			if found {
				return selected
			}
		}
	}
	if projectID != "" {
		project, err := s.projects.Get(projectID)
		if err == nil && project.LastModel.Provider != "" {
			return project.LastModel
		}
	}
	def, err := s.llm.DefaultSelection()
	if err == nil {
		return def
	}
	return llm.Selection{}
}

func (s *service) parseSelection(value string) (llm.Selection, error) {
	parts := strings.Split(value, "\x1f")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return llm.Selection{}, fmt.Errorf("请选择一个完整模型")
	}
	selection := llm.Selection{Provider: parts[0], Model: parts[1], Thinking: parts[2]}
	err := s.llm.Validate(selection)
	if err != nil {
		return llm.Selection{}, err
	}
	return selection, nil
}

func latestModelSelection(events []session.Event) (llm.Selection, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Kind != session.KindModelSelected {
			continue
		}
		var data session.ModelSelectedData
		err := json.Unmarshal(events[index].Data, &data)
		if err != nil {
			return llm.Selection{}, false
		}
		return llm.Selection{Provider: data.Provider, Model: data.Model, Thinking: data.Thinking}, true
	}
	return llm.Selection{}, false
}
