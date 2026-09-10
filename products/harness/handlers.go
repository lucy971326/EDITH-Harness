package harness

import (
	"context"
	"errors"

	"harness/appserver"
)

func (p *Product) handleCreate(_ context.Context, input CreateParams) (SessionResult, error) {
	info, err := p.Create(input.Workspace)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func (p *Product) handleList(_ context.Context, _ ListParams) (ListResult, error) {
	infos, err := p.List()
	if err != nil {
		return ListResult{}, methodError(err)
	}
	result := ListResult{Sessions: make([]SessionView, 0, len(infos))}
	for _, info := range infos {
		result.Sessions = append(result.Sessions, sessionView(info))
	}
	return result, nil
}

func (p *Product) handleGet(_ context.Context, input SessionIDParams) (SessionResult, error) {
	info, err := p.Session(input.SessionID)
	return SessionResult{Session: sessionView(info)}, methodError(err)
}

func sessionView(info SessionInfo) SessionView {
	return SessionView{
		SessionID: info.Meta.ID,
		Title:     info.Meta.Title,
		CreatedAt: info.Meta.CreatedAt,
		Settings:  info.Settings,
	}
}

func methodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrWorkspace) {
		return &appserver.Error{Code: appserver.CodeInvalidParams, Message: "workspace is not available", Cause: err}
	}
	if errors.Is(err, ErrInvalidRunSettings) {
		return &appserver.Error{Code: appserver.CodeInvalidParams, Message: "model, reasoning effort or agent is unavailable", Cause: err}
	}
	// 底层文件缺失不是目标会话不存在，只有产品的明确判断才能映射为未找到。
	if errors.Is(err, ErrSessionNotFound) {
		return &appserver.Error{Code: appserver.CodeNotFound, Message: "session not found", Cause: err}
	}
	return err
}
