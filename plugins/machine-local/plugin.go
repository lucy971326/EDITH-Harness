// Package machinelocal 提供本机上的文件和进程操作。
package machinelocal

import (
	"harness/kernel/host"
)

var _ host.Plugin = (*Plugin)(nil)

// 活对象。machine-local 插件自己的启动状态。
type Plugin struct {
	local *local
}

// New 造本机机器提供者插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "machine-local" }

func (p *Plugin) Start(h *host.Host) error {
	local, err := newLocal()
	if err != nil {
		return err
	}

	p.local = local
	err = h.RegisterService("machine", local)
	if err != nil {
		p.local = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.local = nil
	return nil
}
