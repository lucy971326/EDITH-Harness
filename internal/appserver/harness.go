package appserver

import (
	"context"
	"errors"
	"fmt"

	"harness/internal/conversations"
	"harness/internal/events"
	"harness/internal/runner"
	"harness/internal/session/settings"
	"harness/internal/subagents"
)

const (
	createMethod             = "harness/session/create"
	listMethod               = "harness/session/list"
	archivedListMethod       = "harness/session/archived/list"
	archiveMethod            = "harness/session/archive"
	restoreMethod            = "harness/session/restore"
	deleteSessionMethod      = "harness/session/delete"
	deleteProjectMethod      = "harness/project/delete"
	getMethod                = "harness/session/get"
	updateSettingsMethod     = "harness/session/settings/update"
	forkMethod               = "harness/session/fork"
	sendMethod               = "harness/session/send"
	snapshotMethod           = "harness/session/snapshot"
	subscribeMethod          = "harness/session/subscribe"
	terminalSubscribeMethod  = "harness/run/terminal/subscribe"
	stopMethod               = "harness/session/stop"
	readRunDiffMethod        = "harness/run/diff/read"
	revertRunDiffMethod      = "harness/run/diff/revertFile"
	subagentListMethod       = "harness/subagent/list"
	subagentSubscribeMethod  = "harness/subagent/subscribe"
	subagentSendMethod       = "harness/subagent/send"
	subagentSettingsMethod   = "harness/subagent/settings/update"
	subagentStopMethod       = "harness/subagent/stop"
	readSubagentDiffMethod   = "harness/subagent/run/diff/read"
	revertSubagentDiffMethod = "harness/subagent/run/diff/revertFile"
)

// BindHarness 在监听前接入产品与事件来源，登记 Harness 的对外方法。
func (s *Server) BindHarness(product *conversations.Service, runService *runner.Runner, registry *events.Registry) error {
	if product == nil || runService == nil || registry == nil {
		return fmt.Errorf("appserver: nil harness product, runner or events")
	}
	if s.conversations != nil {
		return fmt.Errorf("appserver: harness already bound")
	}
	s.conversations = product
	s.runner = runService
	s.events = registry
	err := s.registerWorkspaceSelect()
	if err != nil {
		return err
	}
	err = Register(s, createMethod, s.handleCreate)
	if err != nil {
		return err
	}
	err = Register(s, "harness/session/activity/subscribe", s.handleActivitySubscribe)
	if err != nil {
		return err
	}
	err = Register(s, "harness/session/activity/list", s.handleActivityList)
	if err != nil {
		return err
	}
	err = registerSession(s, "harness/session/read", func(input MarkReadParams) string { return input.SessionID }, s.handleMarkRead)
	if err != nil {
		return err
	}
	err = Register(s, listMethod, s.handleList)
	if err != nil {
		return err
	}
	for _, registration := range []struct {
		method  string
		handler func(context.Context, SessionIDParams) (SessionResult, error)
	}{
		{archiveMethod, s.handleArchive}, {restoreMethod, s.handleRestore},
	} {
		if err = registerSession(s, registration.method, func(input SessionIDParams) string { return input.SessionID }, registration.handler); err != nil {
			return err
		}
	}
	if err = Register(s, archivedListMethod, s.handleArchivedList); err != nil {
		return err
	}
	if err = registerSession(s, deleteSessionMethod, func(input SessionIDParams) string { return input.SessionID }, s.handleDeleteSession); err != nil {
		return err
	}
	if err = Register(s, deleteProjectMethod, s.handleDeleteProject); err != nil {
		return err
	}
	err = Register(s, getMethod, s.handleGet)
	if err != nil {
		return err
	}
	err = registerSession(s, updateSettingsMethod, func(input UpdateSettingsParams) string { return input.SessionID }, s.handleUpdateSettings)
	if err != nil {
		return err
	}
	err = registerSession(s, forkMethod, func(input ForkParams) string { return input.SessionID }, s.handleFork)
	if err != nil {
		return err
	}
	err = registerSession(s, sendMethod, func(input SendParams) string { return input.SessionID }, s.handleSend)
	if err != nil {
		return err
	}

	err = Register(s, snapshotMethod, s.handleSnapshot)
	if err != nil {
		return err
	}
	err = Register(s, subscribeMethod, s.handleSubscribe)
	if err != nil {
		return err
	}
	err = Register(s, terminalSubscribeMethod, s.handleTerminalSubscribe)
	if err != nil {
		return err
	}
	err = Register(s, stopMethod, s.handleStop)
	if err != nil {
		return err
	}
	err = Register(s, readRunDiffMethod, s.handleReadRunDiff)
	if err != nil {
		return err
	}
	err = registerSession(s, revertRunDiffMethod, func(input RevertRunDiffParams) string { return input.SessionID }, s.handleRevertRunDiff)
	if err != nil {
		return err
	}
	err = Register(s, subagentListMethod, s.handleSubagentList)
	if err != nil {
		return err
	}
	err = Register(s, subagentSubscribeMethod, s.handleSubagentSubscribe)
	if err != nil {
		return err
	}
	err = registerSession(s, subagentSendMethod, func(input SubagentSendParams) string {
		return input.ParentSessionID + "\x00" + input.TaskID
	}, s.handleSubagentSend)
	if err != nil {
		return err
	}
	err = registerSession(s, subagentSettingsMethod, func(input SubagentSettingsParams) string {
		return input.ParentSessionID + "\x00" + input.TaskID
	}, s.handleSubagentSettings)
	if err != nil {
		return err
	}
	err = Register(s, subagentStopMethod, s.handleSubagentStop)
	if err != nil {
		return err
	}
	err = Register(s, readSubagentDiffMethod, s.handleReadSubagentRunDiff)
	if err != nil {
		return err
	}
	return registerSession(s, revertSubagentDiffMethod, func(input RevertSubagentRunDiffParams) string {
		return input.ParentSessionID + "\x00" + input.TaskID
	}, s.handleRevertSubagentRunDiff)
}

func (s *Server) handleCreate(_ context.Context, input CreateParams) (SessionResult, error) {
	defer s.invalidateActivity()
	info, err := s.conversations.Create(input.Workspace)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleList(_ context.Context, _ ListParams) (ListResult, error) {
	infos, err := s.conversations.List()
	if err != nil {
		return ListResult{}, methodError(err)
	}
	result := ListResult{Sessions: make([]SessionView, 0, len(infos))}
	for _, info := range infos {
		result.Sessions = append(result.Sessions, sessionView(info))
	}
	return result, nil
}

func (s *Server) handleArchivedList(_ context.Context, _ ListParams) (ListResult, error) {
	infos, err := s.conversations.Archived()
	if err != nil {
		return ListResult{}, methodError(err)
	}
	result := ListResult{Sessions: make([]SessionView, 0, len(infos))}
	for _, info := range infos {
		result.Sessions = append(result.Sessions, sessionView(info))
	}
	return result, nil
}

func (s *Server) handleArchive(_ context.Context, input SessionIDParams) (SessionResult, error) {
	defer s.invalidateActivity()
	info, err := s.conversations.Archive(input.SessionID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleRestore(_ context.Context, input SessionIDParams) (SessionResult, error) {
	defer s.invalidateActivity()
	info, err := s.conversations.Restore(input.SessionID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleDeleteSession(_ context.Context, input SessionIDParams) (DeleteResult, error) {
	defer s.invalidateActivity()
	ids, err := s.conversations.DeleteSession(input.SessionID)
	return DeleteResult{SessionIDs: ids}, methodError(err)
}

func (s *Server) handleDeleteProject(_ context.Context, input DeleteProjectParams) (DeleteResult, error) {
	defer s.invalidateActivity()
	ids, err := s.conversations.DeleteProject(input.Workspace)
	return DeleteResult{SessionIDs: ids}, methodError(err)
}

func (s *Server) handleGet(_ context.Context, input SessionIDParams) (SessionResult, error) {
	info, err := s.conversations.Session(input.SessionID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleUpdateSettings(ctx context.Context, input UpdateSettingsParams) (SessionResult, error) {
	info, err := s.conversations.UpdateSettings(ctx, input.SessionID, settings.SessionSettings{
		AgentID:         input.AgentID,
		Model:           input.Model,
		ReasoningEffort: input.ReasoningEffort,
		PermissionMode:  input.PermissionMode,
	})
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (s *Server) handleFork(_ context.Context, input ForkParams) (SessionResult, error) {
	defer s.invalidateActivity()
	destinationID, err := s.conversations.Fork(conversations.ForkInput{
		SessionID:       input.SessionID,
		RunID:           input.RunID,
		BoundaryEntryID: input.BoundaryEntryID,
	})
	if err != nil {
		return SessionResult{}, methodError(err)
	}
	info, err := s.conversations.Session(destinationID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func sessionView(info conversations.SessionInfo) SessionView {
	return SessionView{
		SessionID:  info.Meta.ID,
		Title:      info.Meta.Title,
		CreatedAt:  info.Meta.CreatedAt,
		ArchivedAt: info.Meta.ArchivedAt,
		Settings:   info.Settings,
	}
}

func methodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, conversations.ErrRunChanged) {
		return &Error{Code: CodeConflict, Message: "expected run has ended or changed", Cause: err}
	}
	if errors.Is(err, subagents.ErrTaskNotFound) || errors.Is(err, subagents.ErrOwnershipMismatch) {
		return &Error{Code: CodeNotFound, Message: "subagent task not found", Cause: err}
	}
	if errors.Is(err, subagents.ErrTaskStopped) || errors.Is(err, subagents.ErrFamilyStopped) {
		return &Error{Code: CodeConflict, Message: "subagent task was stopped", Cause: err}
	}
	if errors.Is(err, subagents.ErrTaskActive) {
		return &Error{Code: CodeConflict, Message: "subagent has an active run", Cause: err}
	}
	if errors.Is(err, subagents.ErrInvalidSettings) {
		return &Error{Code: CodeInvalidParams, Message: "subagent model or reasoning effort is unavailable", Cause: err}
	}
	if errors.Is(err, subagents.ErrDescriptionEmpty) || errors.Is(err, subagents.ErrTaskNameEmpty) {
		return &Error{Code: CodeInvalidParams, Message: "subagent input is empty", Cause: err}
	}
	if errors.Is(err, runner.ErrRunDiffNotFound) {
		return &Error{Code: CodeNotFound, Message: "run diff file not found", Cause: err}
	}
	if errors.Is(err, runner.ErrRunDiffConflict) || errors.Is(err, runner.ErrRunDiffActive) {
		return &Error{Code: CodeConflict, Message: "run diff has changed or cannot be reverted now", Cause: err}
	}
	if errors.Is(err, conversations.ErrInvalidMessage) {
		return &Error{Code: CodeInvalidParams, Message: "message is empty", Cause: err}
	}
	if errors.Is(err, conversations.ErrWorkspace) {
		return &Error{Code: CodeInvalidParams, Message: "workspace is not available", Cause: err}
	}
	if errors.Is(err, conversations.ErrInvalidRunSettings) {
		return &Error{Code: CodeInvalidParams, Message: "model, reasoning effort or agent is unavailable", Cause: err}
	}
	if errors.Is(err, conversations.ErrRunActive) {
		return &Error{Code: CodeConflict, Message: "session has an active run", Cause: err}
	}
	if errors.Is(err, conversations.ErrArchived) {
		return &Error{Code: CodeConflict, Message: "restore archived session before starting another run", Cause: err}
	}
	if errors.Is(err, conversations.ErrInvalidCommand) {
		return &Error{Code: CodeInvalidParams, Message: "command is unavailable", Cause: err}
	}
	if errors.Is(err, conversations.ErrCommandRejected) {
		return &Error{Code: CodeConflict, Message: "command cannot run in the current session", Cause: err}
	}
	// 底层文件缺失不是目标会话不存在，只有产品的明确判断才能映射为未找到。
	if errors.Is(err, conversations.ErrSessionNotFound) {
		return &Error{Code: CodeNotFound, Message: "session not found", Cause: err}
	}
	return err
}
