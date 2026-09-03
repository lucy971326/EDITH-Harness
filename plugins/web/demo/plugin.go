// Package demo 提供验证 Web 产品登记处的最小产品插件。
package demo

import (
	nethttp "net/http"

	"github.com/a-h/templ"

	"harness/kernel/host"
	"harness/surface/web"
	"harness/surface/web/ui"
)

var product = web.Product{
	ID:       "demo",
	Name:     "演示",
	Icon:     ui.IconZap,
	BasePath: "/demo",
	Order:    20,
}

// 活对象。Plugin 将演示产品及页面路由填入 Web 登记处。
type Plugin struct {
	web web.Service
}

// New 创建演示产品插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "web-demo" }

func (p *Plugin) Start(h *host.Host) error {
	webService, err := host.Resolve[web.Service](h, "web")
	if err != nil {
		return err
	}
	p.web = webService
	err = webService.RegisterProduct(product)
	if err != nil {
		return err
	}
	return webService.RegisterRoute("GET /demo", p)
}

func (p *Plugin) Close() error {
	return nil
}

func (p *Plugin) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	content := DemoPage()
	if r.Header.Get("HX-Request") == "true" {
		templ.Handler(web.ProductFragment(p.web.Products(), product.ID, templ.NopComponent, content)).ServeHTTP(w, r)
		return
	}
	templ.Handler(web.Page(p.web.Products(), product.ID, templ.NopComponent, content)).ServeHTTP(w, r)
}
