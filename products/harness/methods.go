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
