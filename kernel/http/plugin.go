package http

import (
	"errors"
	"fmt"
	"log/slog"
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

// 活对象。把 HTTP 路径登记处挂到 Host，并管理 HTTP 服务。
type Plugin struct {
	config Config
	server *nethttp.Server
}

// NewPlugin 造 HTTP 插件。
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

func (p *Plugin) Name() string { return "http" }

func (p *Plugin) Start(h *host.Host) error {
	listener, err := listen(p.config)
	if err != nil {
		return err
	}

	registry := newRegistry()
	server := &nethttp.Server{
		Addr:              listener.Addr().String(),
		Handler:           registry,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	err = h.RegisterService("http", registry)
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	p.server = server
	go serve(server, listener)
	return nil
}

func (p *Plugin) Close() error {
	if p.server == nil {
		return nil
	}
	err := p.server.Close()
	p.server = nil
	return err
}

func listen(config Config) (net.Listener, error) {
	if config.Host != loopbackHost && config.Host != allInterfacesHost {
		return nil, fmt.Errorf("http: unsupported host %q", config.Host)
	}
	if config.Port < 0 || config.Port > 65535 {
		return nil, fmt.Errorf("http: invalid port %d", config.Port)
	}

	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("http: listen on %s: %w", address, err)
	}
	return listener, nil
}

func serve(server *nethttp.Server, listener net.Listener) {
	err := server.Serve(listener)
	if errors.Is(err, nethttp.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return
	}
	if err != nil {
		slog.Error("HTTP server stopped", "error", err)
	}
}
