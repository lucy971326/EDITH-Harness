package llm

import "harness/kernel/host"

var _ host.Plugin = (*Plugin)(nil)

// 活对象。启动时读取本机配置，构造并挂上 LLM Client。
type Plugin struct {
	client *Client
}

func (p *Plugin) Name() string { return "llm" }

func (p *Plugin) Start(h *host.Host) error {
	config, err := loadConfig()
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
