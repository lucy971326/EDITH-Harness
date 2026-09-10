package harness

import (
	"harness/appserver"
	"harness/kernel/events"
)

// CreateMethod 创建或复用工作区中的空会话。
func CreateMethod() appserver.Method[CreateParams, SessionResult] {
	return appserver.Method[CreateParams, SessionResult]{Name: "harness/session/create"}
}

// ListMethod 列出普通会话，不包含子会话。
func ListMethod() appserver.Method[ListParams, ListResult] {
	return appserver.Method[ListParams, ListResult]{Name: "harness/session/list"}
}

// GetMethod 查询一场普通会话。
func GetMethod() appserver.Method[SessionIDParams, SessionResult] {
	return appserver.Method[SessionIDParams, SessionResult]{Name: "harness/session/get"}
}

// SendMethod 闲时开始新一轮，忙时插话。
func SendMethod() appserver.Method[SendParams, SendResult] {
	return appserver.Method[SendParams, SendResult]{Name: "harness/session/send"}
}

// SnapshotMethod 读取耐久账本与当前运行。
func SnapshotMethod() appserver.Method[SessionIDParams, Snapshot] {
	return appserver.Method[SessionIDParams, Snapshot]{Name: "harness/session/snapshot"}
}

// SubscribeMethod 订阅运行事件并取得初始快照。
func SubscribeMethod() appserver.Method[SessionIDParams, SubscribeResult] {
	return appserver.Method[SessionIDParams, SubscribeResult]{Name: "harness/session/subscribe"}
}

// StopMethod 停止当前会话及其孩子。
func StopMethod() appserver.Method[SessionIDParams, StopResult] {
	return appserver.Method[SessionIDParams, StopResult]{Name: "harness/session/stop"}
}

func (p *Product) registerMethods(server *appserver.RPCServer, registry *events.Registry) error {
	err := appserver.Register(server, CreateMethod(), p.handleCreate)
	if err != nil {
		return err
	}
	err = appserver.Register(server, ListMethod(), p.handleList)
	if err != nil {
		return err
	}
	err = appserver.Register(server, GetMethod(), p.handleGet)
	if err != nil {
		return err
	}
	handlers := &runHandlers{product: p, events: registry}
	err = appserver.Register(server, SendMethod(), handlers.send)
	if err != nil {
		return err
	}
	err = appserver.Register(server, SnapshotMethod(), handlers.snapshot)
	if err != nil {
		return err
	}
	err = appserver.Register(server, SubscribeMethod(), handlers.subscribe)
	if err != nil {
		return err
	}
	return appserver.Register(server, StopMethod(), handlers.stop)
}
