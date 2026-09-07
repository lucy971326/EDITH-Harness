package harness

import (
	"context"
	"errors"

	"harness/appserver"
)

func CreateMethod() appserver.Method[CreateParams, SessionResult] {
	return appserver.Method[CreateParams, SessionResult]{Name: "harness/session/create", Description: "创建或复用工作区中的空会话"}
}

func ListMethod() appserver.Method[ListParams, ListResult] {
	return appserver.Method[ListParams, ListResult]{Name: "harness/session/list", Description: "列出普通会话，不包含子会话"}
}

func GetMethod() appserver.Method[GetParams, SessionResult] {
	return appserver.Method[GetParams, SessionResult]{Name: "harness/session/get", Description: "查询一场普通会话"}
}

// Definitions 只读取契约声明；生成时不构造产品或启动依赖。
func Definitions() ([]appserver.Definition, error) {
	create, err := appserver.Describe(CreateMethod())
	if err != nil {
		return nil, err
	}
	list, err := appserver.Describe(ListMethod())
	if err != nil {
		return nil, err
	}
	get, err := appserver.Describe(GetMethod())
	if err != nil {
		return nil, err
	}
	return []appserver.Definition{create, get, list}, nil
}

func (p *Product) registerMethods(server *appserver.Server) error {
	err := appserver.Register(server, CreateMethod(), func(ctx context.Context, input CreateParams) (SessionResult, error) {
		info, err := p.Create(input.Workspace)
		return SessionResult{Session: sessionView(info)}, methodError(err)
	})
	if err != nil {
		return err
	}
	err = appserver.Register(server, ListMethod(), func(ctx context.Context, input ListParams) (ListResult, error) {
		infos, err := p.List()
		if err != nil {
			return ListResult{}, methodError(err)
		}
		out := ListResult{Sessions: make([]SessionView, 0, len(infos))}
		for _, info := range infos {
			out.Sessions = append(out.Sessions, sessionView(info))
		}
		return out, nil
	})
	if err != nil {
		return err
	}
	return appserver.Register(server, GetMethod(), func(ctx context.Context, input GetParams) (SessionResult, error) {
		info, err := p.Session(input.SessionID)
		return SessionResult{Session: sessionView(info)}, methodError(err)
	})
}

func sessionView(info SessionInfo) SessionView {
	return SessionView{SessionID: info.Meta.ID, Title: info.Meta.Title, CreatedAt: info.Meta.CreatedAt, Settings: info.Settings}
}

func methodError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrWorkspace) {
		return &appserver.Error{Code: appserver.CodeInvalidParams, Message: "workspace is not available", Cause: err}
	}
	// 底层文件缺失不是目标会话不存在，只有产品的明确判断才能映射为未找到。
	if errors.Is(err, ErrSessionNotFound) {
		return &appserver.Error{Code: appserver.CodeNotFound, Message: "session not found", Cause: err}
	}
	return err
}
