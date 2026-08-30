package persist

import (
	"harness/kernel/agents/config"
	"harness/kernel/host"
	"harness/kernel/session/settings"
)

var (
	_ host.Plugin                   = (*Plugin)(nil)
	_ Persistence                   = (*jsonl)(nil)
	_ settings.SessionSettingsStore = (*jsonl)(nil)
	_ config.Store                  = (*jsonl)(nil)
)

// 活对象。把同一份 jsonl 挂成三把键：账本、会话设置和 Agent 设置。
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
	err = h.RegisterService("agentStore", s)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
