package host

import (
	"errors"
	"fmt"
)

// 活对象。进程那张服务表。插件往上挂，别人 Resolve 取出。
type Host struct {
	services map[string]any
	started  []Plugin
}

// NewHost 造一张空的进程服务表。
func NewHost() *Host {
	return &Host{
		services: make(map[string]any),
	}
}

// RegisterService 把一份活对象挂到名字上。重名、空名、nil 都是组装错误。
func (h *Host) RegisterService(name string, v any) error {
	if name == "" {
		return fmt.Errorf("host: empty service name")
	}
	if v == nil {
		return fmt.Errorf("host: register %q: nil value", name)
	}
	if _, exists := h.services[name]; exists {
		return fmt.Errorf("host: %q already registered", name)
	}
	h.services[name] = v
	return nil
}

// Resolve 按名字取出，并断言成 T。没有或类型不对都返回 error。
func Resolve[T any](h *Host, name string) (T, error) {
	var zero T
	v, ok := h.services[name]
	if !ok {
		return zero, fmt.Errorf("host: %q not registered", name)
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("host: %q type mismatch: want %T, have %T", name, zero, v)
	}
	return typed, nil
}

// Install 调 p.Start。失败则把这个插件 Close，再倒序拆掉已经装上的，并清空服务表。
func (h *Host) Install(p Plugin) error {
	if p == nil {
		return fmt.Errorf("host: install nil plugin")
	}

	err := p.Start(h)
	if err != nil {
		startErr := fmt.Errorf("host: start %s: %w", p.Name(), err)
		closeSelf := p.Close()
		closeRest := h.Close()
		return errors.Join(startErr, closeSelf, closeRest)
	}

	h.started = append(h.started, p)
	return nil
}

// Close 倒序拆已装插件，然后清空服务表。插件 Close 出错也继续拆完。
func (h *Host) Close() error {
	var errs []error
	for i := len(h.started) - 1; i >= 0; i-- {
		p := h.started[i]
		err := p.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("host: close %s: %w", p.Name(), err))
		}
	}
	h.started = nil
	h.services = make(map[string]any)
	return errors.Join(errs...)
}
