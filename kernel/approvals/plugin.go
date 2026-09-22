package approvals

import "harness/kernel/host"

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
	return h.RegisterService("approvals", p.service)
}
func (p *Plugin) Close() error {
	if p.service == nil {
		return nil
	}
	return p.service.Close()
}
