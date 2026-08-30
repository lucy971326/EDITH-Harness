package persist

import (
	"harness/kernel/host"
	"harness/kernel/session/settings"
)

var (
	_ host.Plugin                   = (*Plugin)(nil)
	_ Persistence                   = (*jsonl)(nil)
	_ settings.SessionSettingsStore = (*jsonl)(nil)
)

// 活对象。把同一份 jsonl 挂成两把键：sessionPersistence 和 sessionSettings。
type Plugin struct {
	Dir string
}

func (p *Plugin) Name() string { return "persist" }

func (p *Plugin) Start(h *host.Host) error {
	s, err := openJSONL(p.Dir)
	if err != nil {
		return err
	}
	err = h.RegisterService("sessionPersistence", s)
	if err != nil {
		return err
	}
	err = h.RegisterService("sessionSettings", s)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
