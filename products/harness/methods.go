package harness

import "harness/appserver"

func CreateMethod() appserver.Method[CreateParams, SessionResult] {
	return appserver.Method[CreateParams, SessionResult]{Name: "harness/session/create", Description: "创建或复用工作区中的空会话"}
}

func ListMethod() appserver.Method[ListParams, ListResult] {
	return appserver.Method[ListParams, ListResult]{Name: "harness/session/list", Description: "列出普通会话，不包含子会话"}
}

func GetMethod() appserver.Method[GetParams, SessionResult] {
	return appserver.Method[GetParams, SessionResult]{Name: "harness/session/get", Description: "查询一场普通会话"}
}

func (p *Product) registerMethods(server *appserver.RPCServer) error {
	err := appserver.Register(server, CreateMethod(), p.handleCreate)
	if err != nil {
		return err
	}
	err = appserver.Register(server, ListMethod(), p.handleList)
	if err != nil {
		return err
	}
	return appserver.Register(server, GetMethod(), p.handleGet)
}
