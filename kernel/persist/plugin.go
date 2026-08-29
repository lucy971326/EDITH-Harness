package persist

import (
	"harness/kernel/host"
	"harness/kernel/kinds"
)

var (
	_ host.Plugin  = (*Plugin)(nil)
	_ Persistence  = (*jsonl)(nil)
	_ kinds.Setups = (*jsonl)(nil)
)

// Plugin 把同一份 jsonl 挂成两把键：sessionPersistence 和 setups。
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
	err = h.RegisterService("setups", s)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
