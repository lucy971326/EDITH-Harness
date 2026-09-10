package appserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"harness/appserver"
	"harness/kernel/host"
)

// 测试插件只参与 Host 生命周期，不持有 app-server。
type methodPlugin struct {
	fail   bool
	closes int
}

func (*methodPlugin) Name() string { return "test-product" }
func (p *methodPlugin) Start(_ *host.Host) error {
	if p.fail {
		return errors.New("product assembly failed")
	}
	return nil
}
func (p *methodPlugin) Close() error { p.closes++; return nil }

func TestEntryOwnsServerAndHostOwnsPlugins(t *testing.T) {
	h := host.NewHost()
	s := appserver.New()
	defer s.Close()
	err := appserver.Register(s, "product/get", emptyResult)
	if err != nil {
		t.Fatal(err)
	}
	p := &methodPlugin{}
	err = h.Install(p)
	if err != nil {
		t.Fatal(err)
	}
	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
	if p.closes != 1 {
		t.Fatalf("plugin closed %d times", p.closes)
	}
	// 直接创建的 app-server 不在 Host 内；入口必须自己关。
	_, err = s.Call(context.Background(), "product/get", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal("Host unexpectedly closed entry-owned server", err)
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

func TestFailedProductAssemblyCleanupClosesCalls(t *testing.T) {
	h := host.NewHost()
	s := appserver.New()
	defer s.Close()
	err := appserver.Register(s, "product/get", emptyResult)
	if err != nil {
		t.Fatal(err)
	}
	first, failing := &methodPlugin{}, &methodPlugin{fail: true}
	err = h.Install(first)
	if err != nil {
		t.Fatal(err)
	}
	// 依赖组装错误导致第二个产品安装失败，Host 回滚两个插件。
	err = h.Install(failing)
	if err == nil {
		t.Fatal("failed product installation succeeded")
	}
	if first.closes != 1 || failing.closes != 1 {
		t.Fatal("failed installation was not cleaned up")
	}
	// 入口在组装失败时关闭 app-server，并且不启动网络监听。
	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Call(context.Background(), "product/get", json.RawMessage(`{}`))
	var public *appserver.Error
	if !errors.As(err, &public) || public.Code != appserver.CodeConflict {
		t.Fatal("closed server accepted a call", err)
	}
}

func emptyResult(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil }
