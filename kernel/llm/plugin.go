package llm

import (
	"harness/kernel/host"
	"harness/kernel/persist"
)

// 活对象。启动时读取本机配置，构造并挂上 LLM Client。
type Plugin struct {
	client *Client
}

func (p *Plugin) Name() string { return "llm" }

func (p *Plugin) Start(h *host.Host) error {
	files, err := host.Resolve[*persist.Files](h, "persist")
	if err != nil {
		return err
	}
	config, err := loadConfig(files)
	if err != nil {
		return err
	}
	client, err := newClient(config)
	if err != nil {
		return err
	}

	p.client = client
	err = h.RegisterService("llm", client)
	if err != nil {
		p.client = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.client = nil
	return nil
}
