package session

import (
	"harness/kernel/host"
	"harness/kernel/persist"
)

// 活对象。把 Session Store 挂到 Host 的 sessions 键。
type Plugin struct {
	store *Store
}

func (p *Plugin) Name() string { return "session" }

func (p *Plugin) Start(h *host.Host) error {
	persistence, err := host.Resolve[persist.Persistence](h, "sessionPersistence")
	if err != nil {
		return err
	}
	p.store = NewStore(persistence)
	err = h.RegisterService("sessions", p.store)
	if err != nil {
		p.store = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.store = nil
	return nil
}
