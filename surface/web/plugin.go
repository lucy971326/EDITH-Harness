package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	nethttp "net/http"
	"strconv"
	"time"

	"harness/kernel/host"
)

const (
	loopbackHost      = "127.0.0.1"
	allInterfacesHost = "0.0.0.0"
)

// 活对象。Plugin 持有 Web 登记处和正在运行的 HTTP 服务。
type Plugin struct {
	config Config
	server *nethttp.Server
	done   chan error
}

// NewPlugin 使用监听配置创建 Web 插件。
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

func (p *Plugin) Name() string { return "web" }

// Start 登记 Web 服务并启动 HTTP 服务器。
func (p *Plugin) Start(h *host.Host) error {
	listener, err := listen(p.config)
	if err != nil {
		return err
	}

	registry := newRegistry()
	err = registerWebRoutes(registry)
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	err = h.RegisterService("web", Service(registry))
	if err != nil {
		return errors.Join(err, listener.Close())
	}

	server := &nethttp.Server{
		Addr:              listener.Addr().String(),
		Handler:           registry,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	p.server = server
	p.done = make(chan error, 1)
	go p.serve(listener)
	return nil
}

// URL 返回 Start 成功后的实际本地地址。
func (p *Plugin) URL() string {
	if p.server == nil {
		return ""
	}
	hostName, port, err := net.SplitHostPort(p.server.Addr)
	if err != nil {
		return ""
	}
	if hostName == allInterfacesHost {
		hostName = loopbackHost
	}
	return "http://" + net.JoinHostPort(hostName, port)
}

// Close 关闭 HTTP 服务器并等待 Serve goroutine 退出。
func (p *Plugin) Close() error {
	if p.server == nil {
		return nil
	}
	closeErr := p.server.Close()
	serveErr := <-p.done
	p.server = nil
	p.done = nil
	return errors.Join(closeErr, serveErr)
}

func (p *Plugin) serve(listener net.Listener) {
	err := p.server.Serve(listener)
	if errors.Is(err, nethttp.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		err = nil
	}
	p.done <- err
}

func registerWebRoutes(registry *Registry) error {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("web: static files: %w", err)
	}
	err = registry.RegisterRoute("GET /static/", nethttp.StripPrefix("/static/", nethttp.FileServerFS(staticRoot)))
	if err != nil {
		return err
	}
	err = registry.RegisterRoute("GET /{$}", newRootHandler(registry))
	if err != nil {
		return err
	}
	settingsHandler := newSettingsHandler(registry)
	err = registry.RegisterRoute("GET /settings", settingsHandler)
	if err != nil {
		return err
	}
	err = registry.RegisterRoute("GET /settings/{sectionID}", settingsHandler)
	if err != nil {
		return err
	}
	return nil
}

func listen(config Config) (net.Listener, error) {
	if config.Host != loopbackHost && config.Host != allInterfacesHost {
		return nil, fmt.Errorf("web: unsupported host %q", config.Host)
	}
	if config.Port < 0 || config.Port > 65535 {
		return nil, fmt.Errorf("web: invalid port %d", config.Port)
	}
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("web: listen on %s: %w", address, err)
	}
	return listener, nil
}
