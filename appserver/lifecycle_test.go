package appserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"harness/appserver"
	"harness/kernel/host"
)

// 活对象。验证 Host 生命周期与直接登记的 app-server 不混为一谈。
type methodPlugin struct {
	server *appserver.RPCServer
	closes int
}

func (*methodPlugin) Name() string { return "test-product" }
func (p *methodPlugin) Start(h *host.Host) error {
	var err error
	p.server, err = host.Resolve[*appserver.RPCServer](h, "appServer")
	if err != nil {
		return err
	}
	return appserver.Register(p.server, appserver.Method[struct{}, struct{}]{Name: "product/get"}, func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })
}
func (p *methodPlugin) Close() error { p.closes++; return nil }

func TestEntryOwnsServerAndHostOwnsPlugins(t *testing.T) {
	h := host.NewHost()
	s := appserver.New()
	defer s.Close()
	err := h.RegisterService("appServer", s)
	if err != nil {
		t.Fatal(err)
	}
	p := &methodPlugin{}
	err = h.Install(p)
	if err != nil {
		t.Fatal(err)
	}
	if p.server != s {
		t.Fatal("plugin did not receive entry-owned instance")
	}
	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
	if p.closes != 1 {
		t.Fatalf("plugin closed %d times", p.closes)
	}
	// 直接登记的 RPCServer 没有被 Host 当作插件关闭；入口必须自己关。
	err = s.Freeze()
	if err != nil {
		t.Fatal("Host unexpectedly closed directly registered server", err)
	}
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
	if p.closes != 1 {
		t.Fatal("Host repeated plugin close")
	}
}

func TestFailedProductAssemblyNeverOpensCalls(t *testing.T) {
	h := host.NewHost()
	s := appserver.New()
	defer s.Close()
	err := h.RegisterService("appServer", s)
	if err != nil {
		t.Fatal(err)
	}
	first, failing := &methodPlugin{}, &methodPlugin{}
	err = h.Install(first)
	if err != nil {
		t.Fatal(err)
	}
	// 重复接口导致第二个产品安装失败，Host 回滚两个插件。
	err = h.Install(failing)
	if err == nil {
		t.Fatal("duplicate method installation succeeded")
	}
	if first.closes != 1 || failing.closes != 1 {
		t.Fatal("failed installation was not cleaned up")
	}
	_, err = s.Call(context.Background(), "product/get", json.RawMessage(`{}`))
	var public *appserver.Error
	if !errors.As(err, &public) || public.Code != appserver.CodeConflict {
		t.Fatal("failed assembly accepted a call", err)
	}
	// 与入口失败清理相同：直接关闭 RPCServer，不能重新冻结开放。
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = s.Freeze()
	if err == nil {
		t.Fatal("failed server reopened")
	}
}
