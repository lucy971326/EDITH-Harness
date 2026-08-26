package web

import (
	"fmt"
	"net/http"

	"harness/presets"
)

func (s *service) handlePresetList(writer http.ResponseWriter, request *http.Request) {
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetListPage(data))
}

func (s *service) handleNewPreset(writer http.ResponseWriter, request *http.Request) {
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, presets.Preset{}))
}

func (s *service) handleEditPreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := s.presets.Get(request.URL.Query().Get("id"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusNotFound)
		return
	}
	data, err := s.presetPageData("")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, preset))
}

func (s *service) handleCreatePreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := presetFromForm(request)
	if err == nil {
		err = s.validatePresetChoices(preset)
	}
	if err == nil {
		err = s.presets.Create(preset)
	}
	if err != nil {
		s.renderPresetFormError(writer, request, preset, err)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) handleUpdatePreset(writer http.ResponseWriter, request *http.Request) {
	preset, err := presetFromForm(request)
	if err == nil {
		err = s.validatePresetChoices(preset)
	}
	if err == nil {
		err = s.presets.Update(preset)
	}
	if err != nil {
		s.renderPresetFormError(writer, request, preset, err)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) handleArchivePreset(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err == nil {
		err = s.presets.Archive(request.Form.Get("id"))
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(writer, request, "/presets", http.StatusSeeOther)
}

func (s *service) validatePresetChoices(preset presets.Preset) error {
	if preset.ID == "" {
		return fmt.Errorf("Agent 模式必须有名称")
	}
	installed := make(map[string]bool)
	for _, name := range s.tools.Names() {
		installed[name] = true
	}
	for _, name := range preset.Tools {
		if !installed[name] {
			return fmt.Errorf("工具 %s 没有安装", name)
		}
	}
	return nil
}

func (s *service) renderPresetFormError(writer http.ResponseWriter, request *http.Request, preset presets.Preset, cause error) {
	data, err := s.presetPageData(cause.Error())
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, PresetFormPage(data, preset))
}
