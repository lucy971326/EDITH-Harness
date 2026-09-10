package harness

import (
	"harness/appserver"
	"harness/kernel/events"
)

const (
	createMethod    = "harness/session/create"
	listMethod      = "harness/session/list"
	getMethod       = "harness/session/get"
	sendMethod      = "harness/session/send"
	snapshotMethod  = "harness/session/snapshot"
	subscribeMethod = "harness/session/subscribe"
	stopMethod      = "harness/session/stop"
)

func (p *Product) registerMethods(server *appserver.Server, registry *events.Registry) error {
	err := appserver.Register(server, createMethod, p.handleCreate)
	if err != nil {
		return err
	}
	err = appserver.Register(server, listMethod, p.handleList)
	if err != nil {
		return err
	}
	err = appserver.Register(server, getMethod, p.handleGet)
	if err != nil {
		return err
	}
	err = appserver.Register(server, sendMethod, p.send)
	if err != nil {
		return err
	}

	runs := &runHandlers{product: p, events: registry}
	err = appserver.Register(server, snapshotMethod, runs.snapshot)
	if err != nil {
		return err
	}
	err = appserver.Register(server, subscribeMethod, runs.subscribe)
	if err != nil {
		return err
	}
	return appserver.Register(server, stopMethod, runs.stop)
}
