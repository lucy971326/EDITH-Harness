package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"harness/agents"
	"harness/commands"
)

func (s *service) handleHome(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), request.URL.Query().Get("error"))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, Page(data))
}

func (s *service) handleChatFragment(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) handleChatLogFragment(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, ChatLog(data.Chat))
}

func (s *service) handleComposerFragment(writer http.ResponseWriter, request *http.Request) {
	data, err := s.pageData(request.URL.Query().Get("project"), request.URL.Query().Get("session"), request.URL.Query().Get("draft"), "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	render(writer, request, MessageBox(data))
}

func (s *service) handleSubmitMessage(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "消息表单读取失败", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(request.Form.Get("text"))
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	presetID := request.Form.Get("preset_id")
	if text == "" {
		s.respondMessageError(writer, request, projectID, sessionID, "消息不能为空")
		return
	}
	if commands.LooksLike(text) {
		s.handleCommand(writer, request, projectID, sessionID, text)
		return
	}
	fallback := s.currentOrProjectModel(projectID, sessionID)
	selection, selectionErr := s.parseSelectionFromForm(request.Form, fallback)
	if selectionErr != nil {
		s.respondMessageError(writer, request, projectID, sessionID, selectionErr.Error())
		return
	}

	if sessionID == "" {
		conversation, startErr := s.agents.StartSession(agents.StartInput{
			ProjectID: projectID,
			PresetID:  presetID,
			Title:     titleFromMessage(text),
			Model:     selection,
		})
		if startErr != nil {
			s.respondMessageError(writer, request, projectID, "", startErr.Error())
			return
		}
		sendErr := conversation.SubmitFollowup(text)
		if sendErr != nil {
			s.respondMessageError(writer, request, projectID, conversation.SessionID(), sendErr.Error())
			return
		}
		rememberErr := s.projects.RememberPreset(projectID, presetID)
		if rememberErr != nil {
			fmt.Printf("Web UI：记住项目模式失败：%v\n", rememberErr)
		}
		rememberErr = s.projects.RememberModel(projectID, selection)
		if rememberErr != nil {
			fmt.Printf("Web UI：记住项目模型失败：%v\n", rememberErr)
		}
		sessionID = conversation.SessionID()
		s.redirectAfterNewSession(writer, request, projectID, sessionID)
		return
	}

	conversation, openErr := s.agents.OpenSession(sessionID)
	if openErr != nil {
		s.respondMessageError(writer, request, projectID, sessionID, openErr.Error())
		return
	}
	err = conversation.SelectModel(selection)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	err = s.projects.RememberModel(projectID, selection)
	if err != nil {
		fmt.Printf("Web UI：记住项目模型失败：%v\n", err)
	}
	err = conversation.SubmitFollowup(text)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	data, dataErr := s.pageData(projectID, sessionID, "", "")
	if dataErr != nil {
		http.Error(writer, dataErr.Error(), http.StatusInternalServerError)
		return
	}
	data.Busy = true
	render(writer, request, ChatPanel(data))
}

// handleSelectModel 立刻切换已打开会话或草稿的下一轮模型，并把它记进账本与偏好。
func (s *service) handleSelectModel(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "模型表单读取失败", http.StatusBadRequest)
		return
	}
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	draft := request.Form.Get("draft")
	fallback := s.currentOrProjectModel(projectID, sessionID)
	selection, err := s.parseSelectionFromForm(request.Form, fallback)
	if err == nil && sessionID != "" {
		conversation, openErr := s.agents.OpenSession(sessionID)
		if openErr != nil {
			err = openErr
		} else {
			err = conversation.SelectModel(selection)
		}
	}
	if err != nil {
		if request.Header.Get("HX-Target") == "model-picker" {
			data, dataErr := s.pageData(projectID, sessionID, draft, err.Error())
			if dataErr != nil {
				http.Error(writer, dataErr.Error(), http.StatusInternalServerError)
				return
			}
			render(writer, request, ModelPicker(data))
			return
		}
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	if projectID != "" {
		remErr := s.projects.RememberModel(projectID, selection)
		if remErr != nil {
			fmt.Printf("Web UI：记住项目模型失败：%v\n", remErr)
		}
	}
	data, err := s.pageData(projectID, sessionID, draft, "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	if request.Header.Get("HX-Target") == "model-picker" {
		render(writer, request, ModelPicker(data))
		return
	}
	render(writer, request, ChatPanel(data))
}

func (s *service) handleCancelSession(writer http.ResponseWriter, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		http.Error(writer, "取消表单读取失败", http.StatusBadRequest)
		return
	}
	projectID := request.Form.Get("project_id")
	sessionID := request.Form.Get("session_id")
	conversation, err := s.agents.GetSession(sessionID)
	if err != nil {
		data, dataErr := s.pageData(projectID, sessionID, "", "当前会话没有在运行")
		if dataErr != nil {
			http.Error(writer, dataErr.Error(), http.StatusInternalServerError)
			return
		}
		render(writer, request, MessageBox(data))
		return
	}
	conversation.Cancel()
	conversation.WaitIdle()
	data, err := s.pageData(projectID, sessionID, "", "")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	render(writer, request, MessageBox(data))
}

func (s *service) redirectAfterNewSession(writer http.ResponseWriter, request *http.Request, projectID string, sessionID string) {
	target := "/?project=" + url.QueryEscape(projectID) + "&session=" + url.QueryEscape(sessionID)
	if request.Header.Get("HX-Request") == "true" {
		writer.Header().Set("HX-Redirect", target)
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(writer, request, target, http.StatusSeeOther)
}

func (s *service) handleCommand(writer http.ResponseWriter, request *http.Request, projectID string, sessionID string, text string) {
	result, err := s.commands.Execute(request.Context(), sessionID, text)
	if err != nil {
		s.respondMessageError(writer, request, projectID, sessionID, err.Error())
		return
	}
	draft := ""
	if sessionID == "" {
		draft = "1"
	}
	message := ""
	draftText := ""
	if result.Kind == commands.KindError {
		message = result.Text
		draftText = text
	}
	data, dataErr := s.pageData(projectID, sessionID, draft, message)
	if dataErr != nil {
		http.Error(writer, dataErr.Error(), http.StatusInternalServerError)
		return
	}
	if result.Kind == commands.KindSuccess {
		data.Notice = result.Text
	}
	data.DraftText = draftText
	render(writer, request, ChatPanel(data))
}

func (s *service) respondMessageError(writer http.ResponseWriter, request *http.Request, projectID string, sessionID string, message string) {
	data, err := s.pageData(projectID, sessionID, "1", message)
	if err != nil {
		http.Error(writer, message, http.StatusBadRequest)
		return
	}
	render(writer, request, ChatPanel(data))
}

func titleFromMessage(text string) string {
	text = strings.TrimSpace(text)
	const limit = 28
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}
