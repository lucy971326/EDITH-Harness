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

// 活对象。提供固定文件存储，并挂出账本、会话设置和 Agent 设置契约。
type Plugin struct {
	Dir string
}

func (p *Plugin) Name() string { return "persist" }

func (p *Plugin) Start(h *host.Host) error {
	files, err := NewFiles(p.Dir)
	if err != nil {
		return err
	}
	err = h.RegisterService("persist", files)
	if err != nil {
		return err
	}
	s := newJSONL(files)
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
