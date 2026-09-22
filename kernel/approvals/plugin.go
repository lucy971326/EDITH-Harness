package approvals

import (
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/persist"
	"harness/kernel/session"
)

// 活对象。将独立审批服务登记到 Host。
type Plugin struct {
	service *Service
}

// NewPlugin 创建审批插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}
func (p *Plugin) Name() string {
	return "approvals"
}
func (p *Plugin) Start(h *host.Host) error {
	p.service = New()
	files, err := host.Resolve[*persist.Files](h, "persist")
	if err != nil {
		return err
	}
	p.service.models, err = host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return err
	}
	p.service.sessions, err = host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		return err
	}
	err = p.service.loadSettings(files)
	if err != nil {
		return err
	}
	return h.RegisterService("approvals", p.service)
}
func (p *Plugin) Close() error {
	if p.service == nil {
		return nil
	}
	return p.service.Close()
}
